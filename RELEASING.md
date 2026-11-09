# Releasing

TrackID is versioned with [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
and has not reached `1.0`: the public Go API and the database schema may change
between `0.x` releases without a deprecation period. Releases are tags on
`main`, announced as GitHub releases.

## When to release

Release when a coherent, tested piece of work lands. There is no fixed cadence.
A release only ever needs three things to be true of `main`:

1. CI is green — `go test ./...` with no skips, `govulncheck`, CodeQL, and the
   frontend lint and build.
2. `CHANGELOG.md` has the change summarised under `[Unreleased]`.
3. The next version number is clear from the change (see below).

## Choosing a version

- **Patch** (`0.x.y`): bug fixes only, no new behaviour, no schema or Go API
  change.
- **Minor** (`0.y.0`): new features, or any change to the Go API or the
  database schema. Until `1.0` this is where most releases land.
- **Major** (`1.0.0`): not expected while the project is a research prototype.

## Cutting a release

1. Move the changes from `[Unreleased]` to a new section in `CHANGELOG.md`,
   dated with the release day, and keep the `[Unreleased]` heading for the
   next cycle.
2. Commit that change on `main` with a message like `Release v0.2.0`.
3. Tag and push:

   ```sh
   git tag -a v0.2.0 -m "TrackID v0.2.0"
   git push origin v0.2.0
   ```

4. Create the GitHub release from the tag, using the CHANGELOG section as the
   notes.

## Notes for consumers

- The Go module path is `github.com/rodfileto/trackid`. Downstream
  organizations depend on a tag, not on `main`, so a tag name must be a valid
  module version (`v` prefix).
- Because the database schema can change between `0.x` releases, a release note
  should call out any migration a deployment must run (`cmd/migrate up` applies
  them).
