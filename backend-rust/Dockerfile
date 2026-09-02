# TrackID Rust backend — multi-stage build.
FROM rust:1.98-slim AS builder
WORKDIR /app
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/target/release/trackid-backend /usr/local/bin/trackid-backend
EXPOSE 8000
CMD ["trackid-backend"]
