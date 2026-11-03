## Summary

What this change does and why. Link the issue it addresses if there is one.

## Scope

- [ ] Go and/or frontend code
- [ ] Database schema (migrations) and regenerated sqlc output
- [ ] Documentation

## Checklist

- [ ] `gofmt` clean and `go vet ./...` passes
- [ ] `sqlc diff` is clean (if `db/queries/` or migrations changed)
- [ ] Tests pass locally with no skips (`TRACKID_TEST_DATABASE_URL` and
      `FINGERPRINT_SIDECAR_URL` set — see CONTRIBUTING.md)
- [ ] Frontend `npm run lint` and `npm run build` pass (if frontend changed)
- [ ] CHANGELOG.md updated under `[Unreleased]`

## Notes for reviewers

Anything you want a reviewer to weigh in on. Remember this project is written
with AI assistants under the author's supervision — keep the AI-assistance note
in the commit message as described in CONTRIBUTING.md.
