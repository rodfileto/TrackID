# Deploying TrackID

How to run TrackID for casework on one server, from a release. `docs/production-plan.md` explains
the choices behind it, and `SECURITY.md` what the application does and does not protect by itself.

The stack is in `deploy/`:

| Service | What it is | Reachable from |
|---|---|---|
| `proxy` | nginx: TLS, the sign-in rate limit, security headers | the network (ports 80, 443) |
| `api` | TrackID's API and web app (`trackid`) | the proxy |
| `worker` | background tasks: face embeddings, fingerprint templates, videos, drafts (`worker`) | — |
| `migrate`, `init-storage` | run once at every start, then exit | — |
| `postgres` | PostgreSQL 16 with pgvector and PostGIS | the app services only |
| `redis` | the task queue | the app services only |
| `seaweedfs` | evidence storage (S3), with object lock | the app services only |
| `sourceafis-sidecar` | fingerprint extraction and matching | the app services only |

The data services sit on a network with no route outside and no published port.

## Requirements

- Linux, x86-64, with Docker Engine 24+ and the Compose plugin.
- **CPU and memory:** 8 cores and 16 GB RAM is a comfortable start for a unit's casework. The
  worker's face models run on the CPU unless a GPU is set up (not covered here yet).
- **Disk:** Postgres grows with persons and features (about 3 KB per face embedding), and evidence
  storage with the files themselves. With object lock, stored files stay for the whole retention
  period, so size the disk for it.
- A host name and a TLS certificate for it from the institute's CA, as `tls.crt` (full chain) and
  `tls.key`.

## Install

```sh
git clone --branch <version> https://github.com/rodfileto/trackid.git
cd trackid/deploy
cp .env.example .env
```

Fill in `.env`. Every empty value under "Version and address", "Secrets" and "Evidence storage"
is required. Generate each secret with `openssl rand -hex 32`. Then:

```sh
mkdir -p certs && cp /path/to/fullchain.pem certs/tls.crt && cp /path/to/key.pem certs/tls.key
./fetch-models.sh                # face models into ./models, checked against their SHA-256
docker compose up -d
docker compose ps                # every service running or healthy; migrate and init-storage exited 0
```

To try it out without a certificate, `./make-test-cert.sh <host name>` makes a self-signed one,
valid 30 days. Browsers warn about it. Never use it in production.

Until release images are published, set `TRACKID_VERSION=local` and run `docker compose build`
before `up`.

## Production mode

`deploy/compose.yml` runs the API and the worker with `TRACKID_ENV=production`. Development forgives
a missing setting by turning its feature off. Production instead stops at start, with a message
naming every problem at once, when:

- the database, Redis or object storage settings are missing, or `JWT_SECRET` is shorter than 32
  bytes, or `APP_URL` isn't an `https://` address (the API);
- an integration you configured doesn't come up: object storage or Redis unreachable, the face
  models missing or failing to load, the fingerprint sidecar, the geocoder, the AI mode's settings,
  or Teams with a `SECRETS_KEY` it can't use.

An integration left unconfigured (no language model, say) is still just off. So a typo stops the
deployment where you can see it, rather than leaving a feature quietly broken. `docker compose logs
api worker` shows the reason, and the container restarts until it is fixed.

`GET /ready`, used by the API's healthcheck, reports whether the database, storage and queue answer.
The worker's healthcheck is `worker -check`. Neither is reachable from outside.

## Keeping secrets

`.env` holds the stack's secrets: make it readable by its owner only (`chmod 600 .env`). The app
also reads each secret from a file, as Docker and Kubernetes secrets provide them: set
`<NAME>_FILE` to the file's path instead of `<NAME>`, for `DATABASE_URL`, `REDIS_URL`,
`JWT_SECRET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `SECRETS_KEY`, `ASSIST_API_KEY`, `ASK_API_KEY` and
`TRACKID_SUPERUSER_PASSWORD`. Setting both forms of one is an error. `deploy/compose.yml` builds
`DATABASE_URL` and `REDIS_URL` from `.env`, so a setup with secret files replaces those two in a
compose override file.

## The first account

There is no public sign-up. Create the first administrator (a `SUPER_ADMIN`) from the server;
the password is asked for without being shown:

```sh
docker compose run --rm -it api create-superuser \
  -first-name Ana -last-name Souza -staff-number 0001 -job-title Perita -username ana.souza
```

Then sign in at `https://<SERVER_NAME>` and create the other accounts from the admin pages.

## Before casework

1. **Calibrate the match thresholds** on the institute's own data (`docs/MODELS.md`). The shipped
   ones are not calibrated: `docker compose run --rm api match-threshold -h`.
2. **Load the municipalities** (needed to place occurrences), from the GeoJSON that
   `tools/municipalities/fetch.py` writes:
   `docker compose run --rm -v /path/to/municipalities.geojson:/in.geojson:ro api load-municipalities -file /in.geojson -commit`.
3. **Set a tile server and a geocoder of your own** (`MAP_TILE_URL`, `MAP_TILE_ORIGIN`,
   `GEOCODER_URL`). Otherwise, users' browsers fetch map tiles from OpenStreetMap's public servers,
   and addresses typed into occurrences go to its public geocoder.
4. **Review the model licences** (`docs/MODELS.md`).
5. **Test a backup and a restore** (below) before the first real case.

## Evidence retention

The evidence bucket is created with S3 object lock in COMPLIANCE mode, with a default retention of
`S3_RETENTION_DAYS`. For that many days after a file is stored, nobody can delete or overwrite it:
not an administrator, not whoever holds the storage keys, and not TrackID itself. Overwriting only
adds a new version and the original stays. This is the storage side of the chain of custody,
alongside the SHA-256 each file is recorded with.

- **The period is the institute's policy.** There is no default, and `init-storage` refuses to
  start without one.
- **Changing it later** applies to files stored from then on. Files already stored keep the period
  they were stored with, and COMPLIANCE retention can't be shortened.
- **It can't be undone.** Test data stored in a production bucket stays for the whole period.
  Try the stack with its own `.env` and volumes, never the production bucket.
- **Locking is set when the bucket is created.** `init-storage` refuses an existing bucket without
  object lock rather than store evidence unprotected.
- **The lock prevents destroying a file, not hiding it.** A plain S3 delete, by anyone holding the
  storage keys, adds a "delete marker": the file disappears from listings and TrackID can no longer
  read it, but its locked version stays. Removing the marker brings it back. Keep the storage keys
  as close as the database password, and watch the bucket for delete markers.
- **Disk.** Storage turns read-only when free disk falls below `SEAWEEDFS_MIN_FREE` percent
  (default 5). Uploads then fail and nothing stored is lost. Watch the volume's disk, and grow it
  before that happens.

## Using your own S3 server

An institute that already runs S3 storage (with object lock) can use it instead of `seaweedfs`.
Remove the `seaweedfs` service and the `init-storage` dependency on it from `compose.yml`, and set
`S3_ENDPOINT` for the app services to your server. `init-storage` then creates the bucket there,
with the same lock and retention, provided the keys are allowed to.

## Backups

Two things make up a deployment's data: the Postgres database and the evidence bucket. Back up
both, at the same moment, and keep copies off the server. Until `deploy/backup.sh` exists
(production plan, phase 4), this does it:

```sh
docker compose exec -T postgres pg_dump -U trackid -Fc trackid > trackid-$(date +%F).dump
docker run --rm --network trackid_internal -v "$PWD/backups/s3:/out" \
  -e RCLONE_CONFIG_S_TYPE=s3 -e RCLONE_CONFIG_S_PROVIDER=Other \
  -e RCLONE_CONFIG_S_ENDPOINT=http://seaweedfs:8333 \
  -e RCLONE_CONFIG_S_ACCESS_KEY_ID="$(grep ^S3_ACCESS_KEY= .env | cut -d= -f2)" \
  -e RCLONE_CONFIG_S_SECRET_ACCESS_KEY="$(grep ^S3_SECRET_KEY= .env | cut -d= -f2)" \
  rclone/rclone:1.71.2 copy --checksum "s:$(grep ^S3_BUCKET= .env | cut -d= -f2)" /out
```

Encrypt the copies, which hold biometric data, before they leave the server. **Test a restore**
into a separate stack: a backup that has never been restored is not a backup.

## Upgrading

1. Read the release's CHANGELOG entry, especially any database migration.
2. Take a backup.
3. Set the new `TRACKID_VERSION` in `.env`, then `docker compose pull && docker compose up -d`.
   `migrate` brings the schema up before the API starts.
4. Check `docker compose ps` and sign in.

Migrations only go forward. To go back, restore the backup taken in step 2 with the previous version.

## Behind another proxy

The API believes the client address in `X-Forwarded-For` only from `TRUSTED_PROXIES`, which
`deploy/compose.yml` sets to Docker's own address ranges, where nginx is.

If the institute's own reverse proxy or load balancer sits in front, every sign-in seems to come
from its address, so the sign-in rate limit applies to everyone together. Either have that proxy
limit `/api/auth/login` per client, or set nginx's `real_ip` module to trust it
(`set_real_ip_from`, `real_ip_header X-Forwarded-For`) in `deploy/nginx/templates/`.

## Extending TrackID (organizations with their own server)

An organization that builds its own server on core (`server.Options`) and adds pages
(`src/extensions/`) builds its own image with the same stages as `Dockerfile`:
- the frontend, with its extensions copied over `src/extensions/`;
- its own main package, built with cgo;
- onnxruntime, checked against the same checksum;
- the same slim runtime.

It then uses `deploy/` with `TRACKID_IMAGE` pointing at its image. The database, storage and
sidecar images stay core's.
