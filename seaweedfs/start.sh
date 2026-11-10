#!/bin/sh
# Starts SeaweedFS as TrackID's object storage: one server (master, volume, filer and S3 gateway)
# with one S3 identity, whose keys come from S3_ACCESS_KEY and S3_SECRET_KEY. Used by both
# docker-compose.yml (development) and deploy/compose.yml (production); see
# docs/production-plan.md, decision 11. Extra arguments go to `weed server`.
set -eu

: "${S3_ACCESS_KEY:?S3_ACCESS_KEY is not set}"
: "${S3_SECRET_KEY:?S3_SECRET_KEY is not set}"
# The keys go into JSON unescaped, so keep them to characters that need no escaping
# (openssl rand -hex gives such keys).
for key in "$S3_ACCESS_KEY" "$S3_SECRET_KEY"; do
  case "$key" in
    *[!A-Za-z0-9_.-]*) echo "S3 keys may hold only letters, digits, '_', '.' and '-'" >&2; exit 1 ;;
  esac
done

umask 077
cat > /tmp/s3.json <<JSON
{"identities":[{"name":"trackid","credentials":[{"accessKey":"$S3_ACCESS_KEY","secretKey":"$S3_SECRET_KEY"}],"actions":["Admin","Read","Write","List","Tagging"]}]}
JSON

# weed server allows 8 volumes by default, and each bucket takes several: at 30 GB a volume, that
# caps a server at about 240 GB whatever its disk, and a few buckets exhaust it. -volume.max=0
# sizes the limit from the free disk instead, in volumes of SEAWEEDFS_VOLUME_SIZE_MB (4 GB, so a
# modest disk still gets enough of them). Below SEAWEEDFS_MIN_FREE (percent of the disk), every
# volume turns read-only: uploads fail rather than the disk filling up.
exec weed server -dir=/data -ip.bind=0.0.0.0 -s3 -s3.config=/tmp/s3.json \
  -volume.max=0 \
  -master.volumeSizeLimitMB="${SEAWEEDFS_VOLUME_SIZE_MB:-4096}" \
  -volume.minFreeSpace="${SEAWEEDFS_MIN_FREE:-5}" \
  "$@"
