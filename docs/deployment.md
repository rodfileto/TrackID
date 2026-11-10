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

**Check the images before running them.** Each release's images are signed by the workflow that
built them from the tagged code (`.github/workflows/images.yml`). With
[cosign](https://docs.sigstore.dev/cosign/system_config/installation/):

```sh
for image in trackid trackid-postgres trackid-sourceafis; do
  cosign verify "ghcr.io/rodfileto/$image:<version>" \
    --certificate-identity-regexp '^https://github.com/rodfileto/trackid/\.github/workflows/images\.yml@refs/tags/v' \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com > /dev/null && echo "$image: ok"
done
```

To build the images on the server instead of pulling them, set `TRACKID_VERSION=local` and run
`docker compose build` before `up`.

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

## Reports (WordCraft)

The image doesn't include [WordCraft](https://github.com/storytold/wordcraft), the report editor
and PDF renderer. Without it, reports can be drafted but not edited in the app or issued. A
deployment that issues reports builds WordCraft itself: `wordcraft-cli` for a Linux x86-64 glibc
system, and the web editor with trunk. It then mounts the build into the `api` service with a
compose override, and sets `WORDCRAFT_CLI` (e.g. `/opt/wordcraft/wordcraft-cli`) and
`WORDCRAFT_WEB_DIR` (e.g. `/opt/wordcraft/web`) in `.env`.

This hasn't been tried with `deploy/` yet. The editor is served at `/wordcraft/`, and its page
may need the Content-Security-Policy in `deploy/nginx/templates/` relaxed for that path.

## Using your own S3 server

An institute that already runs S3 storage (with object lock) can use it instead of `seaweedfs`.
Remove the `seaweedfs` service and the `init-storage` dependency on it from `compose.yml`, and set
`S3_ENDPOINT` for the app services to your server. `init-storage` then creates the bucket there,
with the same lock and retention, provided the keys are allowed to. Set `S3_ENDPOINT_FOR_TOOLS`
in `.env` to the same address, for the backup and restore scripts.

## Backups

The database and the evidence bucket make up a deployment's data. `deploy/backup.sh` backs up
both, from `deploy/`, with the stack running:

```sh
./backup.sh
```

It writes into `BACKUP_DIR` (default `./backups`):

- `<date>_<time>/postgres.dump`: the database, with a `manifest.txt` of what was backed up and
  its checksums. The last `BACKUP_KEEP` dumps are kept (default 14).
- `objects/<bucket>/`: every stored file. Shared by every backup, it only grows, like the bucket,
  so each run copies only what is new. It is checked against the bucket at each run.

The database is dumped first. Stored files are only ever added, under the hash of their content,
so the bucket copied afterwards holds every file the dump refers to.

**Encryption.** Set `BACKUP_AGE_RECIPIENT` to an [age](https://age-encryption.org) public key
(`age-keygen -o backup.key` on another machine; `apt install age` here). The dump is then
encrypted as it is written, as `postgres.dump.age`, and never touches the disk in clear. Keep
the private key off this server, and keep a copy of it somewhere a restore can reach.
The `objects` directory holds evidence in clear. Keep `BACKUP_DIR` on an encrypted disk, and copy
it off the server with the institute's encrypted backup tool.

**Every night**, from the deployment's owner's crontab (`crontab -e`):

```cron
30 2 * * *  cd /srv/trackid/deploy && ./backup.sh >> backups/backup.log 2>&1
```

### Restoring, and the restore drill

`deploy/restore.sh` restores a backup into the stack its `deploy/` runs:

```sh
./restore.sh backups/20261110_023000Z --identity /path/to/backup.key
```

It runs these steps in order:
1. Checks the dump against its manifest.
2. Stops the API and the worker.
3. Replaces the database.
4. Copies the stored files into the bucket, locked again from that day.
5. Starts the stack.
6. Ends with `verify-storage` (below), which must report no problems.

**Test a restore before the first real case, and then regularly.** Use a second copy of
`deploy/` with its own `.env`: another `COMPOSE_PROJECT_NAME`, other ports and other secrets.
Never use the production stack for this, since a restore replaces its database. A backup that
has never been restored is not a backup.

## Checking the stored evidence

`verify-storage` reads every file the database refers to and checks it is there. For case
files, which the database records with their SHA-256, it also checks the content is unchanged:

```sh
docker compose run --rm --no-deps api verify-storage              # everything
docker compose run --rm --no-deps api verify-storage -limit 1000  # a quick sample
```

It writes nothing. It names the rows of any file that is `MISSING` or has a `HASH_MISMATCH`, and
exits 1. Run it after every restore and from time to time (weekly from cron, say), and keep its
output with the deployment's records.

**If it reports a problem**, someone with the storage keys has deleted or overwritten a file.
Object lock keeps the original version, so it can be brought back from the moment before:

```sh
key=trackid-evidence/evidence/<case>/<sha256>   # the row's storage_ref
docker compose --profile ops run --rm -T rclone cat --s3-version-at "2026-11-10 19:00:00" "store:$key" > original
sha256sum original                               # must equal the <sha256> in the key
docker compose --profile ops run --rm -T rclone rcat "store:$key" < original
```

Then run `verify-storage` again. Find out how it happened: the storage keys are in `.env`.

## Monitoring

- **The services:** `docker compose ps` shows every service healthy. The API's healthcheck is
  `/ready` (database, storage, queue), and the worker's is `worker -check`.
- **Background work:** `docker compose exec worker worker -queues` lists the tasks waiting,
  running, retrying and failed. A queue that keeps growing means the worker doesn't keep up. Tasks
  failing means `docker compose logs worker` has something to say.
- **Disk:** the volumes `postgres` and `seaweedfs` (`docker system df -v`), and `BACKUP_DIR`.
  Storage turns read-only below `SEAWEEDFS_MIN_FREE` percent free.
- **The certificate:** its expiry (`openssl x509 -enddate -noout -in certs/tls.crt`).
- **Backups:** that `backup.log` ends each night with "done", and that restores are drilled.

## Upgrading

1. Read the release's CHANGELOG entry, especially any database migration.
2. Take a backup: `./backup.sh`.
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
