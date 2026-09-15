#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="$ROOT_DIR/.run"

stop_process() {
  local name="$1"
  local pid_file="$2"

  if [[ ! -f "$pid_file" ]]; then
    return
  fi

  local pid
  pid="$(cat "$pid_file")"
  if kill -0 "$pid" 2>/dev/null; then
    echo "Stopping $name (PID $pid)..."
    kill -- -"$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
    for _ in {1..10}; do
      if ! kill -0 "$pid" 2>/dev/null; then
        break
      fi
      sleep 1
    done
    if kill -0 "$pid" 2>/dev/null; then
      echo "Force stopping $name (PID $pid)..."
      kill -KILL -- -"$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null || true
    fi
  fi
  rm -f "$pid_file"
}

stop_process "Vite frontend" "$RUN_DIR/frontend.pid"
stop_process "Asynq worker" "$RUN_DIR/worker.pid"
stop_process "Go API" "$RUN_DIR/backend.pid"

if command -v docker >/dev/null 2>&1; then
  echo "Stopping TrackID infrastructure..."
  cd "$ROOT_DIR"
  if [[ -n "$(docker compose --profile graph ps -q neo4j 2>/dev/null)" ]]; then
    echo "Neo4j: stopping running container"
    docker compose --profile graph stop postgres redis minio neo4j
  else
    echo "Neo4j: not running, skipping"
    docker compose stop postgres redis minio
  fi
fi

echo "TrackID development stack stopped."
