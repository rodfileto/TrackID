# Plan: a new repository, and a clean schema baseline

Status: decided 2026-11-02. Parts 1 and 2 done locally, uncommitted (see "Progress" at the end);
Part 3 not started.

## Why

GitHub still shows Claude on TrackID although `main` is clean. The reason is the merged pull
requests #4–#17: GitHub keeps a permanent `refs/pull/N/head` for each one, pointing at the
commits as they were before the 2026-10-28 history rewrite. Those commits include three
authored by `Claude <noreply@anthropic.com>` (PR #4/#5, branch
`claude/tactical-case-linking-docs-8x0l07`, 2026-08-13) and many with `Co-Authored-By: Claude`
lines (55 in PR #17's alone). The owner cannot delete `refs/pull/*` or merged PRs.

What is on `main` today is clean:

- 121 commits, every one authored by rodfileto@gmail.com (as `rodfileto` or `Rodrigo`); no
  `Co-Authored-By` trailer anywhere; the `v0.1.0` tag is clean too.
- GitHub's contributors API lists only `rodfileto`.

A new repository that receives only `main` and the tag carries none of the PR refs.

The other repositories:

| Repository | State |
|---|---|
| trackid-vision | clean: no PR refs, no Claude commits, contributors = rodfileto |
| trackid-sim | **same problem**: PRs #1 and #2 hold commits mentioning Claude (14 and 19 lines) |

What the current TrackID repository holds besides code: 0 stars, 0 forks, no issues, no
releases (the `v0.1.0` tag only), no secrets; description, 11 topics, Apache-2.0; CodeQL and
Dependabot from files in `.github/`; four open Dependabot PRs (#18–#21). Recreating it loses
nothing that cannot be set again in minutes.

## Decisions (taken 2026-11-02)

1. **History: start from a single commit.** The new repository begins with one commit holding the
   current tree; the old history stays in the archived repository.
2. **The old repository: rename it `TrackID-archive` and make it private.**
3. **trackid-sim: the same treatment.**
4. **Schema: squash the 18 migrations into one baseline per domain.**
5. **Table names: fix them in the same pass** (below).

### Table renames

Plural snake_case, like the rest of the schema:

| Before | After |
|---|---|
| `person` | `persons` |
| `identity_document` | `identity_documents` |
| `identity_register` | `identity_registers` |
| `identity_file` | `identity_files` |
| `gender` | `genders` |
| `biometricfeature` | `biometric_features` |
| `biometricfeature_id` (columns of `feature_embeddings`, `biometric_templates`) | `biometric_feature_id` |

Constraint and index names follow. `access_log` keeps its name: a log is already a collection.
Columns that name a row of a renamed table (`person_id`, `identity_file_id`) keep theirs. In Go
the generated `db.Biometricfeature` becomes `db.BiometricFeature` and `BiometricfeatureID`
becomes `BiometricFeatureID`; the other model names do not change (sqlc singularizes).

## Part 1 — schema baseline

### What the 18 migrations are now

| Kind | Migrations |
|---|---|
| Domain tables, in dependency order | 001 users, 002 identity, 003 occurrences, 004 cases, 005 features, 006 analyses, 007 decisions, 008 clusters, 016–017 articles |
| Grab bag | **009 reports**: requests and their questions, plus columns on `case_files`, `case_analyses`, `case_traces`, `biometric_decisions`, `analysis_decision_drafts`, `analysis_reports`; the `conclusion_decision` function; `identification_levels`; the `case_files` category check; report versions and deliveries |
| Patches to earlier tables | 010 (category check again, video source on `case_evidences`), 012 (avatar on `users`), 013 (analysis and report type checks), 014 (`inquiry_number` on `occurrences`, `cited_inquiry_number` on requests), 018 (versions on `case_codifications`/`case_traces`) |
| Data migrations, useless on a fresh database | 011 (moves requests to `case_request_cases`), 015 (`opinion_scale_up` rewrites conclusions), 018's backfill of `codification_embeddings` |

So a reader has to follow `case_files` through 004, 009 and 010, `case_analyses` through 006,
009 and 013, and the opinion scale through 009 and 015 to know what a table is.

### Proposed baseline

One file per domain, each table created in its final shape, with its comments:

| File | Contents |
|---|---|
| `001_foundation.sql` | extensions (postgis, vector, pg_trgm); `users` (with avatar); `access_log` |
| `002_identity.sql` | `person`, `identity_document`, `gender` (+ seed), `identity_register`, `identity_file` |
| `003_occurrences.sql` | `municipalities`, `locations`, `occurrences` (with `inquiry_number` and its checks), `occurrence_types` (+ seed), `occurrence_classifications` |
| `004_requests.sql` | `case_requests` (final shape, with `cited_inquiry_number`), `case_request_questions` |
| `005_cases.sql` | `biometric_cases`, `case_request_cases`, `case_files` (final category check, `case_request_id`), `case_evidences` (video source), `case_videos`, `case_traces` (suitability, role, `recognition_codification_id`), `case_codifications` (label, recipe, created_by), `case_codification_points` |
| `006_features.sql` | `biometricfeature`, `feature_embeddings`, `codification_embeddings`, `biometric_templates`, `match_thresholds` |
| `007_analyses.sql` | `case_analyses` (final types, `case_request_id`), `case_analysis_pairs`, `analysis_assignments` + view, `identification_levels`, `conclusion_decision()`, `biometric_decisions` and `analysis_decision_drafts` (with the final opinion scale check), `biometric_decision_sides` view |
| `008_clusters.sql` | `clusters`, `cluster_members`, `cluster_merges` |
| `009_reports.sql` | `analysis_reports` (final types and snapshot check), `analysis_report_versions`, `report_deliveries` |
| `010_articles.sql` | `articles`, revisions, bodies, mentions, templates, the append-only triggers |

The order follows the foreign keys: requests come before cases, since `case_files` and
`case_analyses` reference them and `case_requests` no longer references a case. Each file's
Down drops what it created.

Dropped in the baseline:

- `case_decisions`: no code uses it (its two queries have no callers).
- `opinion_scale_up`/`opinion_scale_down`: only for converting old rows.
- The data copies of 011 and 018.

### How to prove the baseline is the same schema

1. Migrate an empty database with the current 001–018 and dump it with `pg_dump --schema-only`.
2. Migrate another with the baseline and dump it.
3. Diff the two dumps. The only differences allowed are the ones listed above, plus column order
   where a column used to be added by `ALTER` and is now declared in place. Column order matters
   to nothing that names its columns; `SELECT *` is checked as part of this step.
4. Run sqlc. Generated code must not change, apart from removed queries and possibly field
   order in `db/models.go`.
5. Run the full test suite against a test database, the trackid-sim load tests, and a full sim
   load into the dev database.

### Existing databases

Every database is rebuilt from trackid-sim, so the dev database is dropped and recreated, not
migrated: `goose` there is at 17 and the baseline restarts at 001. Also needed:

- Empty the dev MinIO bucket, whose objects belong to the old rows.
- Reload the sim world.
- `cmd/migrate` needs no change.

## Part 2 — code organization (same pass, no behaviour change)

- **Split `db/queries.sql`** (1,848 lines, 234 queries) into `db/queries/<domain>.sql`, with the
  same domains as the migrations. sqlc takes a directory and writes one `.sql.go` per file;
  package `db` and every function name stay, so no caller changes.
- **Remove the 8 unused queries:**
  - `CountBiometricCases`, `ListBiometricCases`, `UpdateBiometricCaseDescription`,
    `ListBiometricCaseIDsByModality`
  - `InsertCaseDecision`, `ListCaseDecisionsByBiometricCase`
  - `CountCaseAnalysesOfType`, `InsertIdentificationLevel`

  "Unused" here means no reference in trackid or trackid-sim.
- **Point `sqlc.yaml` at the new layout.**
- **Not in this pass:** splitting `api/` (5,355 lines, but already one file per area), and adding
  tests to `person`, `analyses` and `embedding`, which have none. They would be separate changes.

## Part 3 — the new repositories

In this order, each GitHub step confirmed at the time:

1. Land Parts 1 and 2 as one commit on local `main`, verified as above.
2. Tag trackid-vision `v0.5.1` (the ffmpeg range fix) and point trackid at it.
3. On GitHub:
   1. Rename `TrackID` to `TrackID-archive` and make it private.
   2. Create a new public `TrackID` with no README, licence or other files.
   3. Push `main` and the `v0.1.0` tag only, not the remote branches or Dependabot branches.
4. Set again:
   - the description and topics
   - Dependabot security updates
   - code scanning (CodeQL runs from the workflow file)
   - private vulnerability reporting (`SECURITY.md` links to it)
   - branch protection on `main`, if wanted
5. Check that the contributors API and the repository page list only `rodfileto`, and that CI
   and CodeQL pass.
6. Do the same for trackid-sim.
7. The module path `github.com/rodfileto/trackid` and every link in `CITATION.cff`,
   `SECURITY.md` and `.github/ISSUE_TEMPLATE/config.yml` stay valid, because the name is the same.

## Risks

- **Column order in tables created by `ALTER`.** Covered by the dump diff and the `SELECT *`
  check.
- **trackid-sim couples to the schema.** It imports `db`, `db/migrations` and fifteen trackid
  packages, and writes raw SQL to about twenty tables. No table is renamed, so only its tests
  and a full load verify it.
- **The old name's redirect.** GitHub redirects a renamed repository's old URL until a new
  repository takes the name; creating the new `TrackID` ends that redirect, which is the intent.

## Progress (2026-11-02)

Done locally, not committed:

- **Baseline:** `db/migrations/001_foundation.sql` … `010_articles.sql` replace the 18 files.
  Columns that used to be added by `ALTER` stay at the end of their tables, so column order did
  not change.
- **Proof:** `pg_dump --schema-only` of the old chain, with the renames applied, against the
  baseline. The only differences are `case_decisions` (dropped), sequence, primary-key and
  foreign-key names that follow the renames, and the position of `biometric_templates` in the dump.
- **Queries:** `db/queries.sql` is split into `db/queries/<domain>.sql` (226 queries, none lost:
  the same lines, sorted, before and after). The 8 unused queries are removed, and `sqlc diff` is
  clean.
- **Renames:** applied to the queries, the Go code, comments, docs, the frontend's comments, and
  trackid-sim's raw SQL.
- **trackid-sim:** `go.mod` raised to Go 1.27.3, which trackid already required, then
  `go mod tidy`.
- **Tests:** trackid's full suite and trackid-sim's (load tests included) pass against fresh
  databases.

Still to do:

- Rebuild the dev database and MinIO bucket from trackid-sim.
- Tag trackid-vision v0.5.1 and point trackid at it.
- Part 3 (the new repositories).
