#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MIGRATIONS_DIR="$ROOT_DIR/backend/internal/graph/migrations"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi
if [[ ! -d "$MIGRATIONS_DIR" ]]; then
  echo "schema migrations directory not found: $MIGRATIONS_DIR" >&2
  exit 1
fi

cd "$ROOT_DIR"
docker compose --profile graph up -d neo4j

echo "Waiting for Neo4j..."
for attempt in {1..30}; do
  if docker compose exec -T neo4j wget --quiet --tries=1 --spider http://localhost:7474 >/dev/null 2>&1; then
    break
  fi
  if [[ "$attempt" -eq 30 ]]; then
    echo "Neo4j did not become ready" >&2
    exit 1
  fi
  sleep 1
done

for migration in "$MIGRATIONS_DIR"/*.cypher; do
  echo "Applying $(basename "$migration")..."
  docker compose exec -T neo4j cypher-shell \
    -u neo4j \
    -p neo4j_dev \
    --non-interactive \
    < "$migration"
done

echo "Neo4j graph schema applied."
