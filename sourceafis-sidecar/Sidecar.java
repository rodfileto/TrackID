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
//	GET  /healthz           200: ok
//
// Templates are SourceAFIS's own serialized format (toByteArray), opaque to
// trackid. Scores are SourceAFIS similarity scores, unbounded above; the
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
import java.util.Base64;
import java.util.List;
import java.util.concurrent.Executors;

public class Sidecar {
    static final int MAX_BODY = 64 << 20;
    static final Gson GSON = new Gson();

    record MatchRequest(String probe, List<String> candidates) {}
    record MatchResponse(double[] scores) {}

    static class BadRequest extends Exception {
        final int status;
        BadRequest(int status, String message) { super(message); this.status = status; }
    }

    public static void main(String[] args) throws IOException {
        int port = Integer.parseInt(System.getenv().getOrDefault("PORT", "8090"));
        var server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/extract", exchange -> handle(exchange, "POST", Sidecar::extract));
        server.createContext("/match", exchange -> handle(exchange, "POST", Sidecar::match));
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
                if (!exchange.getRequestMethod().equals(method))
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

    static FingerprintTemplate template(String b64, String field) throws BadRequest {
        try {
            return new FingerprintTemplate(Base64.getDecoder().decode(b64));
        } catch (RuntimeException e) {
            throw new BadRequest(400, field + ": not a SourceAFIS template: " + e.getMessage());
        }
    }
}
