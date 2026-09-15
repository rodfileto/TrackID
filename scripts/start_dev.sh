#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="$ROOT_DIR/.run"
BACKEND_DIR="$ROOT_DIR"
FRONTEND_DIR="$ROOT_DIR/frontend"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

PORT="${PORT:-8082}"
DATABASE_URL="${DATABASE_URL:-postgres://trackid:trackid_dev@localhost:55432/trackid?sslmode=disable}"
REDIS_URL="${REDIS_URL:-redis://localhost:56379/0}"
JWT_SECRET="${JWT_SECRET:-local-development-secret}"
S3_ENDPOINT="${S3_ENDPOINT:-http://localhost:59000}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-trackid}"
S3_SECRET_KEY="${S3_SECRET_KEY:-trackid_dev_storage}"
S3_BUCKET="${S3_BUCKET:-trackid-local}"
START_NEO4J="${START_NEO4J:-false}"
NEO4J_URL="${NEO4J_URL:-}"
if [[ "$START_NEO4J" == "true" && -z "$NEO4J_URL" ]]; then
  NEO4J_URL="bolt://localhost:57687"
fi

mkdir -p "$RUN_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi
if ! command -v go >/dev/null 2>&1; then
  echo "go is required" >&2
  exit 1
fi
if ! command -v npm >/dev/null 2>&1; then
  echo "npm is required" >&2
  exit 1
fi

cd "$ROOT_DIR"
echo "Starting TrackID infrastructure..."
docker compose up -d postgres redis minio
if [[ "$START_NEO4J" == "true" ]]; then
  docker compose --profile graph up -d neo4j
fi

# Wait on the host's published port (not the container's internal socket), so a
# stale/broken container that never publishes its port fails here instead of at
# the migration step.
PG_HOST_PORT="$(echo "$DATABASE_URL" | sed -n 's#.*@\([^/]*\)/.*#\1#p')"
PG_HOST="${PG_HOST_PORT%:*}"
PG_PORT="${PG_HOST_PORT##*:}"
# pgx resolves "localhost" to 127.0.0.1; use the same for the readiness probe.
[[ "$PG_HOST" == "localhost" ]] && PG_HOST="127.0.0.1"

echo "Waiting for PostgreSQL on ${PG_HOST}:${PG_PORT}..."
for attempt in {1..30}; do
  if (echo > "/dev/tcp/${PG_HOST}/${PG_PORT}") >/dev/null 2>&1; then
    break
  fi
  if [[ "$attempt" -eq 30 ]]; then
    echo "PostgreSQL did not become ready on ${PG_HOST}:${PG_PORT}" >&2
    exit 1
  fi
  sleep 1
done

echo "Applying database migrations..."
(cd "$BACKEND_DIR" && DATABASE_URL="$DATABASE_URL" go run ./cmd/migrate up)

wait_for_url() {
  local name="$1"
  local url="$2"

  for attempt in {1..30}; do
    if curl --silent --fail --max-time 2 "$url" >/dev/null 2>&1; then
      return
    fi
    if [[ "$attempt" -eq 30 ]]; then
      echo "$name did not become ready; check its log" >&2
      exit 1
    fi
    sleep 1
  done
}

if [[ ! -f "$RUN_DIR/backend.pid" ]] || ! kill -0 "$(cat "$RUN_DIR/backend.pid")" 2>/dev/null; then
  echo "Starting Go API on :$PORT..."
  setsid bash -c "cd '$BACKEND_DIR' && exec env PORT='$PORT' DATABASE_URL='$DATABASE_URL' REDIS_URL='$REDIS_URL' JWT_SECRET='$JWT_SECRET' S3_ENDPOINT='$S3_ENDPOINT' S3_ACCESS_KEY='$S3_ACCESS_KEY' S3_SECRET_KEY='$S3_SECRET_KEY' S3_BUCKET='$S3_BUCKET' NEO4J_URL='$NEO4J_URL' go run ./cmd/trackid" >"$RUN_DIR/backend.log" 2>&1 &
  echo $! >"$RUN_DIR/backend.pid"
else
  echo "Go API is already running (PID $(cat "$RUN_DIR/backend.pid"))"
fi
wait_for_url "Go API" "http://127.0.0.1:$PORT/health"

if [[ ! -f "$RUN_DIR/frontend.pid" ]] || ! kill -0 "$(cat "$RUN_DIR/frontend.pid")" 2>/dev/null; then
  echo "Starting Vite frontend..."
  setsid bash -c "cd '$FRONTEND_DIR' && exec npm run dev -- --host 127.0.0.1" >"$RUN_DIR/frontend.log" 2>&1 &
  echo $! >"$RUN_DIR/frontend.pid"
else
  echo "Vite frontend is already running (PID $(cat "$RUN_DIR/frontend.pid"))"
fi
wait_for_url "Vite frontend" "http://127.0.0.1:5173/"

echo "TrackID development stack is running."
echo "Frontend: http://127.0.0.1:5173"
echo "API:      http://127.0.0.1:$PORT"
echo "Logs:     $RUN_DIR/backend.log and $RUN_DIR/frontend.log"
