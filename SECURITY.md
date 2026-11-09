# Security

TrackID handles biometric and forensic data, so we take security seriously. This
document explains how to report a vulnerability, how reports are handled, what is
supported, and what you should know before deploying.

## Reporting a vulnerability

Do **not** open a public issue for a security problem. Report it privately through
GitHub's private reporting:

- [Report a vulnerability](https://github.com/rodfileto/trackid/security/advisories/new)
  (GitHub Security Advisory), or
- email `rodfileto@gmail.com` if you cannot use GitHub.

Please include:

- the affected component (API, worker, importer surface, frontend, database schema);
- the version or commit you tested;
- steps to reproduce, and the impact you believe it has;
- any mitigations or workarounds you found.

Please do **not** include real operational data (biometric samples, evidence,
personally identifying information) in a report. Use synthetic or redacted data.

## What to expect

- The maintainer will acknowledge a report within 5 working days.
- You will be kept informed of progress; we aim to confirm or refute within 30 days.
- Fixes are released for supported versions as a patch, and a security advisory is
  published crediting you (unless you prefer to stay anonymous).
- The project has a single maintainer and no paid support: response is best-effort,
  and there is no bug bounty.

## Supported versions

TrackID is under active development and has no stable release line yet. Security
fixes are made on the default branch (`main`) and released as a new tag. We support:

| Version | Supported |
| --- | --- |
| latest tag | yes |
| `main` | yes (development) |
| any earlier tag | no — upgrade |

## Security model

TrackID is a self-hosted application. The threat model assumes:

- the deployment runs on infrastructure the institute controls (the API, the worker,
  the database, Redis, MinIO, and the fingerprint sidecar are not exposed to the
  Internet);
- TLS termination happens in front of the API (a reverse proxy), not in the app;
- the database and object storage are not directly reachable by end users.

What the application itself enforces:

- no public sign-up — the first `SUPER_ADMIN` is created out of band, and admins
  create every other account;
- role-based access: examiner, coordinator, intelligence analyst, and admin profiles,
  each limited to their own work (`docs/access-profiles-plan.md`);
- blind verification and an access log (`access_log`) recording reads of cases,
  persons, and face search;
- evidence files are kept with their SHA-256.

## What you must do when deploying

The application does not, by itself, provide everything an operational deployment
needs. Before casework use, an institute must:

1. **Generate real secrets.** The values in `.env.example` and `docker-compose.yml`
   are development defaults. Set a strong `JWT_SECRET`, database password, MinIO
   credentials, and Redis password (Redis in the compose file runs without one).
2. **Put the stack behind TLS.** Terminate HTTPS at a reverse proxy and disable
   direct access to the API, MinIO, Redis, and Postgres ports.
3. **Harden the login endpoint** at the proxy (rate limiting) — the application does
   not rate-limit `/api/auth/login`.
4. **Validate the biometric thresholds** on your own data before casework
   (`docs/MODELS.md`). The shipped thresholds are defaults, not calibrated.
5. **Review the model licences** (`docs/MODELS.md`): the SCRFD detector is
   non-commercial only, and geocoding/map data carry ODbL attribution obligations.
6. **Back up and test restores.** `scripts/backup_data.sh` / `restore_data.sh` are
   development helpers; production needs its own backup, retention, and restore
   testing.

## Reporting issues in third-party components

TrackID depends on other projects. If a vulnerability is in a dependency (a Go
module, an npm package, ONNX Runtime, SourceAFIS, or a model), report it to that
project upstream and, if it affects how TrackID uses it, to us as well.

## Credits

We are grateful to everyone who reports security issues responsibly.
