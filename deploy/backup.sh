#!/usr/bin/env bash
# Backs up a TrackID deployment: the database, then the evidence bucket, into BACKUP_DIR (default
# ./backups). Run it from deploy/, on the server, with the stack up; cron runs it nightly
# (docs/deployment.md, "Backups").
#
#   <BACKUP_DIR>/<date>_<time>/postgres.dump[.age]   the database (pg_dump custom format)
#   <BACKUP_DIR>/<date>_<time>/manifest.txt          what was backed up, with checksums
#   <BACKUP_DIR>/objects/<bucket>/                   every stored file, added to at each run
#
# The database goes first: stored files are only ever added, under their content's hash, so the
# bucket copied afterwards holds every file the dump refers to. The objects directory is shared by
# every backup and only grows, like the bucket; the dumps are kept BACKUP_KEEP at a time (14).
#
# With BACKUP_AGE_RECIPIENT set (an age public key, `age-keygen`), the dump is encrypted as it is
# written and never touches the disk in clear. The objects directory is evidence in clear: keep
# BACKUP_DIR on an encrypted disk, and copy it off the server with the institute's encrypted
# backup tool. The age private key must not be on this server.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"

ENV_FILE="${DEPLOY_ENV_FILE:-.env}"
[[ -f "$ENV_FILE" ]] || { echo "no $ENV_FILE: run from a configured deploy/" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a
BACKUP_DIR="${BACKUP_DIR:-./backups}"
BACKUP_KEEP="${BACKUP_KEEP:-14}"
compose=(docker compose --env-file "$ENV_FILE")

if [[ -n "${BACKUP_AGE_RECIPIENT:-}" ]] && ! command -v age >/dev/null; then
  echo "BACKUP_AGE_RECIPIENT is set but age isn't installed (apt install age)" >&2
  exit 1
fi

# Backups hold biometric data: readable by their owner only.
umask 077
mkdir -p "$BACKUP_DIR/objects"
BACKUP_DIR="$(cd "$BACKUP_DIR" && pwd)"
BACKUP_UID="$(id -u)"
BACKUP_GID="$(id -g)"
export BACKUP_DIR BACKUP_UID BACKUP_GID
stamp="$(date -u +%Y%m%d_%H%M%SZ)"
out="$BACKUP_DIR/$stamp"
mkdir "$out"

echo "==> Database"
if [[ -n "${BACKUP_AGE_RECIPIENT:-}" ]]; then
  dump="$out/postgres.dump.age"
  "${compose[@]}" exec -T postgres pg_dump -U trackid -d trackid -Fc | age -r "$BACKUP_AGE_RECIPIENT" > "$dump"
else
  dump="$out/postgres.dump"
  "${compose[@]}" exec -T postgres pg_dump -U trackid -d trackid -Fc > "$dump"
fi
echo "    $(du -h "$dump" | cut -f1)"

echo "==> Evidence bucket $S3_BUCKET"
"${compose[@]}" --profile ops run --rm -T rclone copy --checksum "store:$S3_BUCKET" "/backup/objects/$S3_BUCKET"
"${compose[@]}" --profile ops run --rm -T rclone check --one-way "store:$S3_BUCKET" "/backup/objects/$S3_BUCKET"
objects="$(find "$BACKUP_DIR/objects/$S3_BUCKET" -type f | wc -l)"
bytes="$(du -sb "$BACKUP_DIR/objects/$S3_BUCKET" | cut -f1)"

cat > "$out/manifest.txt" <<MANIFEST
created_at=$(date -u +%FT%TZ)
trackid_version=${TRACKID_VERSION}
bucket=${S3_BUCKET}
dump=$(basename "$dump")
dump_sha256=$(sha256sum "$dump" | cut -d' ' -f1)
dump_encrypted_to=${BACKUP_AGE_RECIPIENT:-none}
objects_dir=objects/${S3_BUCKET}
objects=${objects}
objects_bytes=${bytes}
MANIFEST
echo "    $objects files, $bytes bytes, matching the bucket"

echo "==> Keeping the last $BACKUP_KEEP dumps"
find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name '20*_*Z' | sort | head -n "-$BACKUP_KEEP" | while read -r old; do
  echo "    removing $(basename "$old")"
  rm -rf "$old"
done

echo "==> Backup $stamp done: $out"
