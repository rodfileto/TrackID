#!/usr/bin/env bash
# One-time move of the development stack's evidence from MinIO to SeaweedFS, which replaced it in
# docker-compose.yml (docs/production-plan.md, decision 11). It starts MinIO on its old volume for a
# moment, copies the bucket into SeaweedFS with rclone, compares the two, and stops MinIO. Nothing
# is deleted: the old volume stays until you remove it yourself (docker volume rm <name>).
#
#   scripts/migrate_minio_to_seaweedfs.sh            # volume trackid_trackid-minio
#   scripts/migrate_minio_to_seaweedfs.sh <volume>   # another project's
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

VOLUME="${1:-trackid_trackid-minio}"
S3_ENDPOINT="${S3_ENDPOINT:-http://localhost:59000}"
S3_ACCESS_KEY="${S3_ACCESS_KEY:-trackid}"
S3_SECRET_KEY="${S3_SECRET_KEY:-trackid_dev_storage}"
S3_BUCKET="${S3_BUCKET:-trackid-local}"
MINIO_TEMP_PORT=59900
MINIO_CONTAINER=trackid-minio-migration

docker volume inspect "$VOLUME" >/dev/null 2>&1 || { echo "no volume $VOLUME" >&2; exit 1; }

echo "==> Starting SeaweedFS ..."
(cd "$ROOT_DIR" && docker compose up -d --wait seaweedfs)

echo "==> Starting MinIO on $VOLUME (port $MINIO_TEMP_PORT, until this script ends) ..."
docker run -d --rm --name "$MINIO_CONTAINER" -p "127.0.0.1:$MINIO_TEMP_PORT:9000" \
  -e MINIO_ROOT_USER=trackid -e MINIO_ROOT_PASSWORD=trackid_dev_storage \
  -v "$VOLUME:/data" minio/minio:latest server /data >/dev/null
trap 'docker stop "$MINIO_CONTAINER" >/dev/null 2>&1 || true' EXIT
for _ in $(seq 1 30); do
  curl -sf "http://127.0.0.1:$MINIO_TEMP_PORT/minio/health/live" >/dev/null && break
  sleep 1
done

rclone() {
  docker run --rm --network host \
    -e RCLONE_CONFIG_OLD_TYPE=s3 -e RCLONE_CONFIG_OLD_PROVIDER=Minio \
    -e "RCLONE_CONFIG_OLD_ENDPOINT=http://127.0.0.1:$MINIO_TEMP_PORT" \
    -e RCLONE_CONFIG_OLD_ACCESS_KEY_ID=trackid -e RCLONE_CONFIG_OLD_SECRET_ACCESS_KEY=trackid_dev_storage \
    -e RCLONE_CONFIG_NEW_TYPE=s3 -e RCLONE_CONFIG_NEW_PROVIDER=Other \
    -e "RCLONE_CONFIG_NEW_ENDPOINT=$S3_ENDPOINT" \
    -e "RCLONE_CONFIG_NEW_ACCESS_KEY_ID=$S3_ACCESS_KEY" -e "RCLONE_CONFIG_NEW_SECRET_ACCESS_KEY=$S3_SECRET_KEY" \
    rclone/rclone:1.71.2 "$@"
}

echo "==> Buckets in MinIO:"
rclone lsd old:
for bucket in $(rclone lsd old: | awk '{print $NF}'); do
  echo "==> Copying $bucket ..."
  rclone copy --checksum "old:$bucket" "new:$bucket"
  rclone check "old:$bucket" "new:$bucket"
done
echo "==> Done. SeaweedFS holds every bucket MinIO had; $VOLUME is untouched."
