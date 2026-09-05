#!/usr/bin/env bash
# Stops the local dev environment started by start_dev.sh: the
# docker-compose stack (postgres/redis/memgraph/minio/rust-backend/ml-sidecar)
# plus any stray frontend or backend process left running outside Docker (e.g.
# a manually-run `npm run dev` or `cargo run`).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Stopping docker-compose stack (docker-compose.dev.yml)..."
docker compose -f docker-compose.dev.yml down

for port_desc in "5173:frontend dev server" "8000:backend dev server" "8001:ml sidecar"; do
    port="${port_desc%%:*}"
    desc="${port_desc#*:}"
    if lsof -ti:"$port" > /dev/null 2>&1; then
        echo "==> Stopping stray $desc on port $port..."
        kill "$(lsof -ti:"$port")" 2>/dev/null || true
    fi
done

echo "==> Done. Data in named volumes (postgres_data, memgraph_data, etc.) is preserved."
