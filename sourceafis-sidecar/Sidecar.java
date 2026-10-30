// Sidecar exposes SourceAFIS for Java (Apache-2.0, 3.18.1) over HTTP, so
// trackid's Go code can extract and match fingerprint templates without a JVM
// in any Go build. trackid talks to it through the fingerprint package; a
// pure-Go port can later replace it behind that same interface.
//
// Endpoints:
//
//	POST /extract?dpi=500   body: image bytes (PNG, JPEG, BMP, WSQ, ...)
//	                        200: template bytes (application/octet-stream)
//	                        422: image SourceAFIS can't decode or extract from
//	POST /match             body: {"probe": b64, "candidates": [b64, ...]}
//	                        200: {"scores": [float, ...]} in candidate order
//	POST /gallery           body: {"templates": [{"key": k, "template": b64}, ...]}
//	                        200: {"size": n}, the templates kept deserialised under their keys
//	GET  /gallery           200: {"size": n}
//	DELETE /gallery         200: {"size": 0}
//	POST /match-cached      body: {"probe": k, "candidates": [k, ...]}
//	                        200: {"scores": [float, ...]} in candidate order
//	                        409: {"missing": [k, ...]}, keys not in the gallery: send them, retry
//	GET  /healthz           200: ok
//
// Templates are SourceAFIS's own serialized format (toByteArray), opaque to
// trackid.
//
// The gallery keeps templates deserialised: rebuilding one costs more than a
// comparison (129 vs 90 microseconds on trackid-sim's prints), and /match does it
// for every candidate of every probe. /match-cached also spreads one probe's
// candidates over every core. Keys are the client's (trackid hashes the template
// bytes, so a changed template is a new key). The gallery is in memory only and
// unbounded, about 26 KB of heap per template; a restart empties it, and the
// client refills it from the 409s. Scores are SourceAFIS similarity scores, unbounded above; the
// library's documented threshold for a match is 40.
//
// Usage (Java 11+ runs a single source file directly):
//
//	./fetch-deps.sh
//	java -cp 'lib/*' Sidecar.java          # listens on $PORT, default 8090

import com.google.gson.Gson;
import com.google.gson.JsonParseException;
import com.machinezoo.sourceafis.FingerprintImage;
import com.machinezoo.sourceafis.FingerprintImageOptions;
import com.machinezoo.sourceafis.FingerprintMatcher;
import com.machinezoo.sourceafis.FingerprintTemplate;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.URLDecoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Executors;
import java.util.concurrent.ForkJoinPool;
import java.util.stream.IntStream;

public class Sidecar {
    static final int MAX_BODY = 64 << 20;
    static final Gson GSON = new Gson();

    record MatchRequest(String probe, List<String> candidates) {}
    record MatchResponse(double[] scores) {}
    record GalleryItem(String key, String template) {}
    record GalleryRequest(List<GalleryItem> templates) {}
    record GallerySize(int size) {}
    record Missing(List<String> missing) {}

    static final ConcurrentHashMap<String, FingerprintTemplate> GALLERY = new ConcurrentHashMap<>();
    static final ForkJoinPool POOL = new ForkJoinPool(Runtime.getRuntime().availableProcessors());

    static class BadRequest extends Exception {
        final int status;
        BadRequest(int status, String message) { super(message); this.status = status; }
    }

    public static void main(String[] args) throws IOException {
        int port = Integer.parseInt(System.getenv().getOrDefault("PORT", "8090"));
        var server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/extract", exchange -> handle(exchange, "POST", Sidecar::extract));
        server.createContext("/match", exchange -> handle(exchange, "POST", Sidecar::match));
        server.createContext("/match-cached", exchange -> handle(exchange, "POST", Sidecar::matchCached));
        server.createContext("/gallery", exchange -> handle(exchange, null, Sidecar::gallery));
        server.createContext("/healthz", exchange -> handle(exchange, "GET", e -> "ok\n".getBytes(StandardCharsets.UTF_8)));
        server.setExecutor(Executors.newFixedThreadPool(Runtime.getRuntime().availableProcessors()));
        server.start();
        System.err.println("sourceafis-sidecar listening on :" + port);
    }

    interface Handler { byte[] apply(HttpExchange exchange) throws Exception; }

    static void handle(HttpExchange exchange, String method, Handler handler) throws IOException {
        try (exchange) {
            int status = 200;
            byte[] body;
            try {
                if (method != null && !exchange.getRequestMethod().equals(method))
                    throw new BadRequest(405, "method not allowed");
                body = handler.apply(exchange);
            } catch (BadRequest e) {
                status = e.status;
                body = (e.getMessage() + "\n").getBytes(StandardCharsets.UTF_8);
            } catch (Exception e) {
                status = 500;
                body = (e + "\n").getBytes(StandardCharsets.UTF_8);
                e.printStackTrace();
            }
            exchange.sendResponseHeaders(status, body.length);
            exchange.getResponseBody().write(body);
        }
    }

    static byte[] readBody(HttpExchange exchange) throws IOException, BadRequest {
        byte[] body = exchange.getRequestBody().readNBytes(MAX_BODY + 1);
        if (body.length > MAX_BODY)
            throw new BadRequest(413, "body larger than " + MAX_BODY + " bytes");
        if (body.length == 0)
            throw new BadRequest(400, "empty body");
        return body;
    }

    static byte[] extract(HttpExchange exchange) throws Exception {
        double dpi = 500;
        String query = exchange.getRequestURI().getRawQuery();
        if (query != null)
            for (String kv : query.split("&")) {
                String[] p = kv.split("=", 2);
                if (p.length == 2 && p[0].equals("dpi")) {
                    try {
                        dpi = Double.parseDouble(URLDecoder.decode(p[1], StandardCharsets.UTF_8));
                    } catch (NumberFormatException e) {
                        throw new BadRequest(400, "dpi: " + e.getMessage());
                    }
                    if (!(dpi >= 20 && dpi <= 20000))
                        throw new BadRequest(400, "dpi out of range: " + dpi);
                }
            }
        byte[] image = readBody(exchange);
        FingerprintTemplate template;
        try {
            template = new FingerprintTemplate(new FingerprintImage(image, new FingerprintImageOptions().dpi(dpi)));
        } catch (IllegalArgumentException | IllegalStateException e) {
            throw new BadRequest(422, "extract: " + e.getMessage());
        }
        exchange.getResponseHeaders().set("Content-Type", "application/octet-stream");
        return template.toByteArray();
    }

    static byte[] match(HttpExchange exchange) throws Exception {
        MatchRequest req;
        try {
            req = GSON.fromJson(new String(readBody(exchange), StandardCharsets.UTF_8), MatchRequest.class);
        } catch (JsonParseException e) {
            throw new BadRequest(400, "json: " + e.getMessage());
        }
        if (req == null || req.probe() == null || req.candidates() == null)
            throw new BadRequest(400, "probe and candidates are required");
        var matcher = new FingerprintMatcher(template(req.probe(), "probe"));
        double[] scores = new double[req.candidates().size()];
        for (int i = 0; i < scores.length; i++)
            scores[i] = matcher.match(template(req.candidates().get(i), "candidates[" + i + "]"));
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        return GSON.toJson(new MatchResponse(scores)).getBytes(StandardCharsets.UTF_8);
    }

    static byte[] gallery(HttpExchange exchange) throws Exception {
        switch (exchange.getRequestMethod()) {
            case "GET" -> {}
            case "DELETE" -> GALLERY.clear();
            case "POST" -> {
                GalleryRequest req;
                try {
                    req = GSON.fromJson(new String(readBody(exchange), StandardCharsets.UTF_8), GalleryRequest.class);
                } catch (JsonParseException e) {
                    throw new BadRequest(400, "json: " + e.getMessage());
                }
                if (req == null || req.templates() == null)
                    throw new BadRequest(400, "templates are required");
                for (GalleryItem item : req.templates())
                    if (item == null || item.key() == null || item.template() == null)
                        throw new BadRequest(400, "each template needs a key and a template");
                // Deserialising is the costly part: on every core.
                var parsed = POOL.submit(() -> req.templates().parallelStream()
                        .map(item -> new FingerprintTemplate(Base64.getDecoder().decode(item.template())))
                        .toList()).get();
                for (int i = 0; i < parsed.size(); i++)
                    GALLERY.put(req.templates().get(i).key(), parsed.get(i));
            }
            default -> throw new BadRequest(405, "method not allowed");
        }
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        return GSON.toJson(new GallerySize(GALLERY.size())).getBytes(StandardCharsets.UTF_8);
    }

    static byte[] matchCached(HttpExchange exchange) throws Exception {
        MatchRequest req;
        try {
            req = GSON.fromJson(new String(readBody(exchange), StandardCharsets.UTF_8), MatchRequest.class);
        } catch (JsonParseException e) {
            throw new BadRequest(400, "json: " + e.getMessage());
        }
        if (req == null || req.probe() == null || req.candidates() == null)
            throw new BadRequest(400, "probe and candidates are required");
        var missing = new ArrayList<String>();
        FingerprintTemplate probe = GALLERY.get(req.probe());
        if (probe == null)
            missing.add(req.probe());
        var candidates = new FingerprintTemplate[req.candidates().size()];
        for (int i = 0; i < candidates.length; i++)
            if ((candidates[i] = GALLERY.get(req.candidates().get(i))) == null)
                missing.add(req.candidates().get(i));
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        if (!missing.isEmpty())
            throw new Conflict(GSON.toJson(new Missing(missing)));
        var matcher = new FingerprintMatcher(probe);
        double[] scores = POOL.submit(() -> IntStream.range(0, candidates.length).parallel()
                .mapToDouble(i -> matcher.match(candidates[i])).toArray()).get();
        return GSON.toJson(new MatchResponse(scores)).getBytes(StandardCharsets.UTF_8);
    }

    // Conflict is a 409 whose body is JSON, not a message line.
    static class Conflict extends BadRequest {
        Conflict(String json) { super(409, json); }
    }

    static FingerprintTemplate template(String b64, String field) throws BadRequest {
        try {
            return new FingerprintTemplate(Base64.getDecoder().decode(b64));
        } catch (RuntimeException e) {
            throw new BadRequest(400, field + ": not a SourceAFIS template: " + e.getMessage());
        }
    }
}
