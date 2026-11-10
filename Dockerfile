# TrackID's application image: the API (`trackid`, the default command), the worker (`worker`)
# and every other command under cmd/ (`migrate`, `init-storage`, `create-superuser`, ...), with the
# frontend the API serves and the CPU build of onnxruntime the vision package loads.
# docs/deployment.md runs it with deploy/compose.yml.
#
# Not in the image: the model weights (mounted at /models, see deploy/fetch-models.sh and
# docs/MODELS.md), the configuration, and anything the deployment stores.

ARG GO_VERSION=1.27.3
ARG NODE_VERSION=24.20.0
ARG DEBIAN_VERSION=bookworm-20261020

# --- The frontend --------------------------------------------------------------------------------
FROM node:${NODE_VERSION}-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# --- The Go commands -----------------------------------------------------------------------------
# cgo is on: the vision package loads onnxruntime through it.
FROM golang:${GO_VERSION}-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# --- onnxruntime, checked against the release it pins ------------------------------------------
# The version onnxruntime_go (go.mod) targets; trackid-vision's scripts/fetch-runtime-deps.sh pins
# the same one. Bump the two together.
# curl and sha256sum rather than ADD --checksum, so the classic builder (Docker without buildx)
# builds it too.
FROM golang:${GO_VERSION}-bookworm AS onnxruntime
ARG ORT_VERSION=1.22.0
ARG ORT_SHA256=8344d55f93d5bc5021ce342db50f62079daf39aaafb5d311a451846228be49b3
RUN curl -fsSL -o /tmp/ort.tgz \
        https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-x64-${ORT_VERSION}.tgz \
    && echo "${ORT_SHA256}  /tmp/ort.tgz" | sha256sum -c - \
    && tar -xzf /tmp/ort.tgz -C /tmp \
    && mkdir -p /opt/onnxruntime \
    && cp /tmp/onnxruntime-linux-x64-${ORT_VERSION}/lib/libonnxruntime.so.${ORT_VERSION} /opt/onnxruntime/libonnxruntime.so

# --- The runtime ---------------------------------------------------------------------------------
FROM debian:${DEBIAN_VERSION}-slim
# ffmpeg/ffprobe: the video package's frames; curl: the API's healthcheck in deploy/compose.yml.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl ffmpeg tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --user-group --home-dir /app --no-create-home trackid

COPY --from=onnxruntime /opt/onnxruntime /opt/onnxruntime
COPY --from=backend /out/ /usr/local/bin/
COPY --from=frontend /src/frontend/dist /app/web

# Defaults for the image's layout; deploy/.env sets everything else. The vision paths name the
# default, permissively licensed models (docs/MODELS.md) that deploy/fetch-models.sh downloads.
ENV PORT=8082 \
    TRACKID_WEB_DIST=/app/web \
    VISION_SHARED_LIBRARY_PATH=/opt/onnxruntime/libonnxruntime.so \
    VISION_DETECTOR=yunet \
    VISION_DETECTOR_PATH=/models/face_detection_yunet_2026may.onnx \
    VISION_RECOGNIZER_PATH=/models/glintr100.onnx

WORKDIR /app
USER 10001:10001
EXPOSE 8082
CMD ["trackid"]

LABEL org.opencontainers.image.title="TrackID" \
      org.opencontainers.image.source="https://github.com/rodfileto/trackid" \
      org.opencontainers.image.licenses="Apache-2.0"
