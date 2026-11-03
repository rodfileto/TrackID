#!/usr/bin/env bash
# Restores a folder produced by scripts/backup_data.sh into the Postgres and
# S3/MinIO instances pointed to by DATABASE_URL / S3_* (from .env, or override
# via env vars for a prod target, e.g.:
#   DATABASE_URL=postgres://... S3_ENDPOINT=... S3_ACCESS_KEY=... \
#   S3_SECRET_KEY=... S3_BUCKET=... scripts/restore_data.sh /path/to/backup
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

IN_DIR="${1:?usage: restore_data.sh <backup-folder> [--yes]}"
AUTO_YES="${2:-}"

DATABASE_URL="${DATABASE_URL:-postgres://trackid:trackid_dev@localhost:55432/trackid?sslmode=disable}"
S3_ENDPOINT="${S3_ENDPOINT:-http://localhost:59000}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-trackid}"
S3_SECRET_KEY="${S3_SECRET_KEY:-trackid_dev_storage}"
S3_BUCKET="${S3_BUCKET:-trackid-local}"

if [[ ! -f "$IN_DIR/postgres.dump" ]]; then
  echo "no postgres.dump found in $IN_DIR" >&2
  exit 1
fi

S3_SRC_DIR="$(find "$IN_DIR/s3" -mindepth 1 -maxdepth 1 -type d | head -n1 || true)"

echo "This will OVERWRITE data at:"
echo "  Postgres: $DATABASE_URL"
echo "  S3:       $S3_ENDPOINT bucket '$S3_BUCKET'"
if [[ "$AUTO_YES" != "--yes" ]]; then
  read -r -p "Continue? [y/N] " confirm
  [[ "$confirm" == "y" || "$confirm" == "Y" ]] || { echo "aborted"; exit 1; }
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

echo "==> Restoring PostgreSQL from $IN_DIR/postgres.dump ..."
docker run --rm --network host \
  -v "$IN_DIR/postgres.dump:/backup/postgres.dump:ro" \
  postgres:16-alpine \
  pg_restore --clean --if-exists --no-owner --no-privileges \
  --dbname="$DATABASE_URL" /backup/postgres.dump

if [[ -n "$S3_SRC_DIR" ]]; then
  echo "==> Restoring S3 bucket '$S3_BUCKET' from $S3_SRC_DIR ..."
  S3_HOST_NO_SCHEME="${S3_ENDPOINT#http://}"
  S3_HOST_NO_SCHEME="${S3_HOST_NO_SCHEME#https://}"
  docker run --rm --network host \
    -e HOME=/tmp \
    -e "MC_HOST_restoredst=${S3_ENDPOINT%%://*}://${S3_ACCESS_KEY}:${S3_SECRET_KEY}@${S3_HOST_NO_SCHEME}" \
    -v "$S3_SRC_DIR:/in:ro" \
    --entrypoint /bin/sh \
    minio/mc:latest \
    -c "mc mb --ignore-existing restoredst/$S3_BUCKET && mc mirror --overwrite /in restoredst/$S3_BUCKET"
else
  echo "No S3 backup folder found under $IN_DIR/s3 — skipping S3 restore."
fi

echo "==> Restore complete."
