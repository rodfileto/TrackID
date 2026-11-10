#!/usr/bin/env bash
# Restores a backup made by backup.sh into the stack this deploy/ runs: the database (replacing
# what is there), then the stored files, then checks every file the database refers to
# (verify-storage). Run it from deploy/, on the server.
#
#   ./restore.sh <backup dir>/<date>_<time> [--identity <age key file>] [--yes]
#
# For a restore drill, run it in a copy of deploy/ with its own .env (another COMPOSE_PROJECT_NAME,
# ports and BACKUP_DIR pointing at the backups), never against the production stack: it replaces
# the database. The bucket is created with object lock as usual (init-storage), so the restored
# files are locked again from the day they are restored.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

backup="" identity="" yes=false
while (($#)); do
  case "$1" in
    --identity) identity="$2"; shift 2 ;;
    --yes) yes=true; shift ;;
    *) backup="$1"; shift ;;
  esac
done
[[ -n "$backup" && -f "$backup/manifest.txt" ]] || { echo "usage: $0 <backup dir> [--identity <age key file>] [--yes]" >&2; exit 1; }
backup="$(cd "$backup" && pwd)"
backup_dir="$(dirname "$backup")"

ENV_FILE="${DEPLOY_ENV_FILE:-.env}"
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a
compose=(docker compose --env-file "$ENV_FILE")
manifest() { grep "^$1=" "$backup/manifest.txt" | cut -d= -f2-; }

dump="$backup/$(manifest dump)"
source_bucket="$(manifest bucket)"
[[ "$(sha256sum "$dump" | cut -d' ' -f1)" == "$(manifest dump_sha256)" ]] || { echo "$dump doesn't match its manifest" >&2; exit 1; }
if [[ "$dump" == *.age ]]; then
  [[ -n "$identity" ]] || { echo "the dump is encrypted: give --identity <age key file>" >&2; exit 1; }
  command -v age >/dev/null || { echo "age isn't installed (apt install age)" >&2; exit 1; }
fi

echo "This replaces the database of compose project '${COMPOSE_PROJECT_NAME:-trackid}' with the backup"
echo "of $(manifest created_at) (TrackID $(manifest trackid_version)), and adds its files to bucket $S3_BUCKET."
if ! $yes; then
  read -r -p "Continue? [y/N] " answer
  [[ "$answer" == y || "$answer" == Y ]] || { echo "aborted"; exit 1; }
fi

echo "==> Stopping the API and the worker"
"${compose[@]}" stop api worker proxy >/dev/null 2>&1 || true
"${compose[@]}" up -d --wait postgres redis seaweedfs
"${compose[@]}" run --rm -T --no-deps init-storage

echo "==> Database"
"${compose[@]}" exec -T postgres psql -q -U trackid -d postgres \
  -c "DROP DATABASE IF EXISTS trackid WITH (FORCE)" -c "CREATE DATABASE trackid OWNER trackid"
if [[ "$dump" == *.age ]]; then
  age -d -i "$identity" "$dump" | "${compose[@]}" exec -T postgres pg_restore -U trackid -d trackid --no-owner
else
  "${compose[@]}" exec -T postgres pg_restore -U trackid -d trackid --no-owner < "$dump"
fi

echo "==> Stored files"
BACKUP_DIR="$backup_dir"
BACKUP_UID="$(id -u)"
BACKUP_GID="$(id -g)"
export BACKUP_DIR BACKUP_UID BACKUP_GID
"${compose[@]}" --profile ops run --rm -T rclone copy --checksum "/backup/$(manifest objects_dir)" "store:$S3_BUCKET"
if [[ "$source_bucket" != "$S3_BUCKET" ]]; then
  echo "    note: the backup's bucket was $source_bucket; references to it name that bucket"
fi

echo "==> Starting the stack (migrate brings the schema up to this version)"
"${compose[@]}" up -d --wait

echo "==> Checking every stored file against the database"
"${compose[@]}" run --rm -T --no-deps api verify-storage
echo "==> Restore done"
