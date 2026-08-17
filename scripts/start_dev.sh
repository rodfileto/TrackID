#!/usr/bin/env bash
# Starts the recommended local dev environment for TrackID:
# postgres + redis + backend in Docker, frontend locally with HMR.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Stopping any existing dev environment..."
docker compose -f docker-compose.dev.yml down
if lsof -ti:5173 > /dev/null 2>&1; then
    echo "==> Stopping stray frontend dev server on port 5173..."
    kill "$(lsof -ti:5173)" 2>/dev/null || true
fi

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
    echo "==> Frontend dev server stopped. Docker services (postgres/redis/backend) are still running."
    echo "    They'll be stopped automatically next time you run this script, or run:"
    echo "    docker compose -f docker-compose.dev.yml down"
}
trap cleanup EXIT

echo "==> Starting frontend dev server (http://localhost:5173)..."
cd "$ROOT_DIR/frontend"
npm run dev
