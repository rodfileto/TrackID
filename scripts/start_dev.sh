#!/usr/bin/env bash
# Starts the recommended local dev environment for TrackID:
# postgres + redis + backend in Docker, frontend locally with HMR.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Starting infra + backend (docker-compose.dev.yml)..."
docker compose -f docker-compose.dev.yml up -d

echo "==> Waiting for backend to become healthy on http://localhost:8000 ..."
for _ in $(seq 1 30); do
    if curl -sf http://localhost:8000/docs > /dev/null 2>&1; then
        break
    fi
    sleep 1
done

if [ ! -d "$ROOT_DIR/frontend/node_modules" ]; then
    echo "==> Installing frontend dependencies..."
    (cd "$ROOT_DIR/frontend" && npm install)
fi

cleanup() {
    echo
    echo "==> Stopping frontend dev server. Docker services (postgres/redis/backend) are still running."
    echo "    Run 'docker compose -f docker-compose.dev.yml down' to stop them."
}
trap cleanup EXIT

echo "==> Starting frontend dev server (http://localhost:5173)..."
cd "$ROOT_DIR/frontend"
npm run dev
