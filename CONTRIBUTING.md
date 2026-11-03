# Contributing to TrackID

Thanks for your interest. TrackID is a research prototype in active development
by a single maintainer, so the contribution process is deliberately light. This
document tells you how to report problems, set up development, and get changes
in.

## Ways to help

- **Report a bug** — open an issue with what you ran, what you expected, and what
  happened. Include the version (git tag or commit) you tested.
- **Ask or discuss** — use issues for design and usage questions before writing a
  large change.
- **Fix or build** — small, focused pull requests are easiest to review. If a
  change is large or changes behaviour, open an issue first to agree on the
  approach.

Security problems are handled differently: see [SECURITY.md](SECURITY.md) — do not
open a public issue for them.

## Code of conduct

Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). By
contributing you agree to abide by it.

## Development setup

Prerequisites: Docker, Go (the version in `go.mod`), and Node.js 22 (see
`frontend/package.json`).

```sh
scripts/start_dev.sh   # starts Postgres, Redis, MinIO, (optionally) the sidecar,
                       # migrates, and runs the API, worker, and frontend
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#development) for details: report
editing needs WordCraft built beside the repo, video needs `ffmpeg`, and there is
no public sign-up — create the first account with `cmd/create-superuser`.

## Conventions

- **Go** code is formatted with `gofmt` and passes `go vet ./...`.
- **sqlc** is the source of truth for the generated `db/*.sql.go` and
  `db/models.go`. Queries live in `db/queries/`, one file per domain; change them or
  `db/migrations/`, then regenerate — CI runs `sqlc diff` and fails if the generated
  code is stale. The pinned version is in
  [`.github/workflows/ci.yml`](.github/workflows/ci.yml).
- **Migrations** are goose SQL files in `db/migrations/`, applied with
  `cmd/migrate up`, one file per domain. They need pgvector and PostGIS; use the
  image built from `db/postgres/`.
- **Frontend** code passes `npm run lint` and `npm run build` (run from
  `frontend/`).

## Testing

The database-backed tests create and drop a throwaway database per test, and skip
when `TRACKID_TEST_DATABASE_URL` is unset; the fingerprint tests need the real
sidecar (`FINGERPRINT_SIDECAR_URL`). CI runs everything and fails if any test is
skipped, so run the same way before submitting:

```sh
docker compose up -d --build postgres sourceafis-sidecar
TRACKID_TEST_DATABASE_URL='postgres://trackid:trackid_dev@localhost:55432/trackid?sslmode=disable' \
FINGERPRINT_SIDECAR_URL='http://localhost:58090' \
  go test ./...
```

`TRACKID_TEST_DATABASE_URL` must point at the PostGIS-capable image.

## Pull requests

- Branch from `main`, keep changes focused, and describe *why* in the PR body.
- Make sure CI is green: gofmt, `go vet`, `sqlc diff`, `go test ./...` with no
  skips, and the frontend lint and build.
- New features should come with tests where the surrounding package has them.

## Commit messages and AI assistance

Commits are written as short imperative sentences describing the change (see
`git log`). This project is developed with the help of AI coding assistants
(Claude Code and DeepSeek) under the author's supervision; commits made with an
assistant end with a note of what in them was done with it, so keep the
attribution when you amend or build on one.

## Releasing

Versioning and the release process (tags, CHANGELOG, notes) are in
[RELEASING.md](RELEASING.md).

## Licence

By contributing you agree that your contribution is licensed under the
[Apache-2.0](LICENSE) terms of this repository. Review [docs/MODELS.md](docs/MODELS.md)
before adding or switching models, detectors, or data sources — several have
licence terms that constrain where they can be used.
