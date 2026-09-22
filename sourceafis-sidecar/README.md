# sourceafis-sidecar

[SourceAFIS for Java](https://sourceafis.machinezoo.com/) 3.18.1 (Apache-2.0) behind a
plain `com.sun.net.httpserver.HttpServer` -- no Spring Boot, no build tool. trackid's
`fingerprint` package (`../fingerprint/`) is the Go client; it's the only intended
caller, but the HTTP boundary has no trackid-specific assumptions.

Dependencies are fetched from Maven Central by `fetch-deps.sh` (no Maven install
needed) into `lib/`, which is gitignored -- run it once locally, or let the Dockerfile
run it during the image build.

## Endpoints

- `POST /extract?dpi=500` -- body: raw image bytes (PNG, JPEG, BMP, WSQ, ...).
  Returns the SourceAFIS template (`FingerprintTemplate.toByteArray()`) as
  `application/octet-stream`. 422 if SourceAFIS can't decode the image or find a
  fingerprint in it (not worth retrying). `dpi` defaults to 500, the trackid-sim
  fixtures' resolution.
- `POST /match` -- body: `{"probe": "<base64 template>", "candidates": ["<base64
  template>", ...]}`. Returns `{"scores": [<float>, ...]}`, one SourceAFIS
  similarity score per candidate, in order. Scores are unbounded above; SourceAFIS's
  documented match threshold is 40 (a false match rate of 0.01%).
- `GET /healthz` -- 200 `ok`.

## Running it

```sh
./fetch-deps.sh
PORT=8090 java -cp 'lib/*' Sidecar.java   # Java 11+ runs a single source file directly
```

Or via docker-compose, as the `sourceafis-sidecar` service (host port 58090 by
default -- see `FINGERPRINT_SIDECAR_URL` in `../.env.example`).

## Manual test against the trackid-sim fixtures

`../trackid-sim/tools/fpgen/out/impressions/` has 100 generated fingers' rolled and
latent impressions, already scored against this same SourceAFIS version by
`../trackid-sim/tools/fpscore/Score.java` (`scores.csv`). A quick sanity check:

```sh
curl --data-binary @../../trackid-sim/tools/fpgen/out/impressions/f000_r0.png \
  'localhost:58090/extract?dpi=500' -o r0.tpl
curl --data-binary @../../trackid-sim/tools/fpgen/out/impressions/f000_l0.png \
  'localhost:58090/extract?dpi=500' -o l0.tpl
printf '{"probe":"%s","candidates":["%s"]}' \
  "$(base64 -w0 r0.tpl)" "$(base64 -w0 l0.tpl)" | curl -d @- localhost:58090/match
# expect a score near scores.csv's genuine_lr f000_l0,f000_r0 row (71.57)
```

`../fingerprint/client_test.go` (`TestExtractMatch`) automates this against three of
those fixtures; run it with `FINGERPRINT_SIDECAR_URL=http://localhost:58090 go test
./fingerprint/...` once the sidecar is up.
