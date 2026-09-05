# ---- Build stage: compile the server binary ----
# onnxruntime_go's cgo shim dlopens libonnxruntime.so at runtime rather than
# linking against it at build time, so the builder only needs a C compiler,
# not CUDA or onnxruntime itself.
FROM golang:1.27.1-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /out/server ./cmd/server

# ---- Runtime stage ----
# CUDA 12 + cuDNN 9 runtime libraries, matching what the onnxruntime GPU
# build below needs (libcudart.so.12, libcublas.so.12, libcudnn.so.9, ...).
FROM nvidia/cuda:12.6.3-cudnn-runtime-ubuntu22.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# GPU-enabled onnxruntime: bundles the CUDA execution provider .so the
# CPU-only apt package (libonnxruntime1.23) doesn't ship.
ARG ONNXRUNTIME_VERSION=1.23.2
RUN mkdir -p /app/onnxruntime-gpu \
    && curl -L -o /tmp/ort.tgz \
      "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/onnxruntime-linux-x64-gpu-${ONNXRUNTIME_VERSION}.tgz" \
    && tar -xzf /tmp/ort.tgz --strip-components=2 \
      -C /app/onnxruntime-gpu "onnxruntime-linux-x64-gpu-${ONNXRUNTIME_VERSION}/lib/" \
    && rm /tmp/ort.tgz

# AuraFace-v1 (Apache-2.0) detection + recognition weights.
RUN mkdir -p /app/models \
    && curl -L -o /app/models/scrfd_10g_bnkps.onnx \
      "https://huggingface.co/fal/AuraFace-v1/resolve/main/scrfd_10g_bnkps.onnx" \
    && curl -L -o /app/models/glintr100.onnx \
      "https://huggingface.co/fal/AuraFace-v1/resolve/main/glintr100.onnx"

COPY --from=builder /out/server /app/server

WORKDIR /app
ENV VISION_DETECTOR_MODEL_PATH=/app/models/scrfd_10g_bnkps.onnx \
    VISION_RECOGNIZER_MODEL_PATH=/app/models/glintr100.onnx \
    ONNXRUNTIME_SHARED_LIBRARY_PATH=/app/onnxruntime-gpu/libonnxruntime.so \
    LD_LIBRARY_PATH=/app/onnxruntime-gpu

EXPOSE 8000
CMD ["/app/server"]
