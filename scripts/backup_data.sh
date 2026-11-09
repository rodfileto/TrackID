#!/usr/bin/env bash
# Dumps Postgres and mirrors the S3/MinIO bucket into a single, self-contained,
# timestamped folder that can be copied to another machine and fed to
# restore_data.sh there.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

DATABASE_URL="${DATABASE_URL:-postgres://trackid:trackid_dev@localhost:55432/trackid?sslmode=disable}"
S3_ENDPOINT="${S3_ENDPOINT:-http://localhost:59000}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-trackid}"
S3_SECRET_KEY="${S3_SECRET_KEY:-trackid_dev_storage}"
S3_BUCKET="${S3_BUCKET:-trackid-local}"

OUT_DIR="${1:-$ROOT_DIR/backups/$(date +%Y%m%d_%H%M%S)}"
mkdir -p "$OUT_DIR/s3/$S3_BUCKET"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

echo "==> Dumping PostgreSQL ($DATABASE_URL) ..."
# Custom format: compressed, portable across pg versions/platforms, restorable
# with pg_restore (including --create / --clean / parallel jobs).
docker run --rm --network host \
  postgres:16-alpine \
  pg_dump --format=custom --no-owner --no-privileges --dbname="$DATABASE_URL" \
  > "$OUT_DIR/postgres.dump"
echo "    -> $OUT_DIR/postgres.dump ($(du -h "$OUT_DIR/postgres.dump" | cut -f1))"

echo "==> Mirroring S3 bucket '$S3_BUCKET' from $S3_ENDPOINT ..."
S3_HOST_NO_SCHEME="${S3_ENDPOINT#http://}"
S3_HOST_NO_SCHEME="${S3_HOST_NO_SCHEME#https://}"
docker run --rm --network host \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e "MC_HOST_backupsrc=${S3_ENDPOINT%%://*}://${S3_ACCESS_KEY}:${S3_SECRET_KEY}@${S3_HOST_NO_SCHEME}" \
  -v "$OUT_DIR/s3/$S3_BUCKET:/out" \
  --entrypoint mc \
  minio/mc:latest \
  mirror --overwrite "backupsrc/$S3_BUCKET" /out

echo "    -> $OUT_DIR/s3/$S3_BUCKET ($(du -sh "$OUT_DIR/s3/$S3_BUCKET" | cut -f1))"

cat > "$OUT_DIR/manifest.txt" <<EOF
created_at=$(date -u +%FT%TZ)
source_bucket=$S3_BUCKET
postgres_dump=postgres.dump (pg_dump custom format)
EOF

echo "==> Backup complete: $OUT_DIR"
echo "Copy this folder to the target machine and run scripts/restore_data.sh <folder> there."
