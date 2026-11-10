# Models, data and their licences


Face detection and embedding run through [trackid-vision](https://github.com/rodfileto/trackid-vision)
(see its README for the full table). The code here is not bound by these terms; the
weights are, and they are not bundled — you download them.

| Model | Role | Terms |
|---|---|---|
| YuNet `face_detection_yunet_2026may.onnx` (OpenCV Zoo) | detector, **default** (`VISION_DETECTOR=yunet`) | MIT. Trained on WIDER Face (CC BY-NC-ND); whether that reaches trained weights is unsettled, and the author states no restriction. |
| AuraFace-v1 `glintr100.onnx` (fal) | recognizer | Apache-2.0; fal describes the training data as commercially and publicly available, without naming it. |
| SCRFD `scrfd_10g_bnkps.onnx` (InsightFace) | detector, opt-in (`VISION_DETECTOR=scrfd`) | **Non-commercial research only** (InsightFace's terms). Do not use in a commercial product or service without their licence. |
| [SourceAFIS](https://sourceafis.machinezoo.com/) 3.18.1 (machinezoo) | fingerprint extraction + matching, via `sourceafis-sidecar/` | Apache-2.0. A library, not downloaded weights -- pinned in `sourceafis-sidecar/fetch-deps.sh`, which fetches it and its runtime dependencies from Maven Central at build time. |

The AI mode ([ask-plan.md](ask-plan.md)) and the assist lab bundle no language model: each deployment
chooses its own server and model, under that model's terms. An outside provider's terms also govern what is
sent to it, which is why the AI mode only sends it synthetic data.

Notes for anyone deploying this:

- **Thresholds are yours to validate.** `faceMatchThreshold` and the detector's score
  threshold are defaults, not calibrated on any operational data. A laboratory should
  validate them on its own data before casework use. `fingerprint.MatchThreshold` (40)
  is SourceAFIS's own documented threshold, not one calibrated here either.
- **Embeddings from different detectors are not comparable** (the same face aligned by
  SCRFD vs YuNet had cosine 0.88 on average). Re-embed everything when switching.
- Terms change; check each model's source before relying on this table.

## Maps

The intelligence pages (`/intelligence/map`) draw with [MapLibre GL JS](https://maplibre.org/)
(BSD-3-Clause, pinned in `frontend/package.json`). The base map is OpenStreetMap data (ODbL):
every map must credit "© OpenStreetMap contributors", which the map's attribution control
does. By default the tiles come from OpenStreetMap's own servers, whose
[tile usage policy](https://operations.osmfoundation.org/policies/tiles/) allows light use
only: a deployment sets `MAP_TILE_URL` (an XYZ template, `{z}/{x}/{y}`) and, if its
provider requires it, `MAP_TILE_ATTRIBUTION` on the server, which serves them to the frontend
at `/api/config` (`VITE_MAP_TILE_*` still set a build's default). The same applies to
geocoding (`geocode/`, see `.env.example`).

## What the published images contain

The images published from a release tag (`.github/workflows/images.yml`) hold TrackID's own code
(Apache-2.0) and third-party software under its own licence. No model weights and no data are in
them: `deploy/fetch-models.sh` downloads the weights on the deployment's server.

| Image | Third-party software | Licences |
|---|---|---|
| `trackid` | Debian 12 base; FFmpeg (Debian's build); ONNX Runtime 1.22.0 (Microsoft's release); the Go and npm modules in `go.sum` and `frontend/package-lock.json`; the Outfit font | Debian packages under their own licences, FFmpeg LGPL-2.1+/GPL-2+, ONNX Runtime MIT, Outfit SIL OFL 1.1 |
| `trackid-postgres` | the pgvector image (PostgreSQL 16, pgvector); PostGIS from PGDG | PostgreSQL licence, pgvector PostgreSQL licence, PostGIS GPL-2+ |
| `trackid-sourceafis` | Eclipse Temurin JRE 21; SourceAFIS and its dependencies (`sourceafis-sidecar/fetch-deps.sh`) | GPL-2 with Classpath Exception, Apache-2.0 |

The GPL and LGPL components are unmodified binaries from their distributions: their source is
available from Debian (`apt-get source <package>`, or sources.debian.org at the version the
image's SBOM lists) and from PGDG (apt.postgresql.org). Each image's SBOM, attached to it at
publication, lists every package and version.
