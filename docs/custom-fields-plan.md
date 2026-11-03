# Organization fields: users and biometric cases

Planned 2026-10-29, not yet decided. An organization deploying TrackID will want fields the core
does not have: on its users (a unit, a professional registration) and on its biometric cases (a
police report number, a body number in a DVI). This plan lets it declare those fields without
changing the core schema, and have them show in the screens, the API and the laudo.

## What is there now

- **The extension contract** (`docs/ARCHITECTURE.md`, "Extension contract") gives an organization
  import commands, extra routes through `api.Dependencies.Register`, and a copy of the base
  frontend. It says nothing about extra fields.
- **Free-form JSONB exists only on the KNOWN side**: `person.meta` and `identity_registers.meta`
  (`002_identity.sql`). The upsert replaces it whole (`db/queries/identity.sql`, `meta = EXCLUDED.meta`),
  and no screen knows its keys.
- **`users`** (`001_foundation.sql`) has fixed fields: names, `staff_number`, `job_title`, email,
  role and capability flags, plus the avatar (`012`). `job_title` is free text.
- **`biometric_cases`** (`005_cases.sql`) has `case_type`, `modality`, `description`,
  the TrackID number and the occurrence link. Anything else about a case is in its description,
  its files or its occurrence.
- **Adding a column is safe for the core**: the queries name their columns (no `SELECT *`, no
  `INSERT` without a column list). But migrations are numbered in one sequence (`001`…`013`), so
  an organization's `014_*.sql` would collide with the next core migration.

## Proposed model

Two layers, for two kinds of need.

### 1. Declared fields (the usual case)

Descriptive data, typed and validated by the core and shown by the core screens, with no code
written by the organization.

- **Values**: one column per extended table, `extra JSONB NOT NULL DEFAULT '{}'`, on `users` and
  `biometric_cases`. A GIN index (`jsonb_path_ops`) for filtering later.
  - JSONB rather than a values table: imports write it in the same row, the report snapshot and
    the custody ledger's `to_jsonb(NEW)` carry it with no extra join, and a case is read in one
    query.
- **Definitions**: a `field_definitions` table.

  | Column | Meaning |
  |---|---|
  | `entity` | `USER` or `BIOMETRIC_CASE` (more later: occurrence, request) |
  | `key` | slug, unique per entity, **never changes** (it is the JSON key) |
  | `labels` | `{"pt-BR": "...", "en": "..."}`, like the frontend's i18n |
  | `type` | `TEXT`, `LONG_TEXT`, `NUMBER`, `DATE`, `BOOLEAN`, `SELECT`, `MULTI_SELECT`; **never changes** |
  | `options` | for the select types: `[{code, labels, active}]`; an option is deactivated, never removed |
  | `required` | enforced on create and on edit, not on rows written before the field existed |
  | `case_type`, `modality` | for case fields: NULL means every case; otherwise the field applies only to that type and/or modality |
  | `visibility` | see "Who sees a field" |
  | `in_report` | offered to the laudo's data (see "Reports") |
  | `section`, `sequence` | grouping and order on the screens |
  | `active` | a deactivated field leaves the forms; its stored values stay and show read-only |

- **Where definitions come from**: a file kept in the organization's own distribution
  (`fields.yaml`), loaded by `cmd/fields -file fields.yaml -commit`. That follows the `cmd/`
  dry-run convention, versions the definitions with the organization's code, and gives the same
  fields in test, staging and production. The command refuses to change a key or a type, and
  deactivates rather than deletes. An admin screen for editing them can come later. The file stays
  the source.
- **Validation** lives in a public package, `fields/` (`fields.Validate(entity, scope, extra)`).
  The API calls it on every write, and the organization's import commands call the same function.
  It rejects unknown keys, wrong types and inactive options. The database does not enforce
  definitions: the import commands write into the core tables directly, so they must call
  `Validate`.
- **Writes merge**: a PATCH carries only the keys it changes, and `null` removes a key. This
  avoids the replace-whole problem `meta` has.

### 2. The organization's own tables (the escape hatch)

For data that needs foreign keys, joins, its own indexes or its own logic (e.g. a table of
units with their hierarchy that users point at), the organization keeps its own migrations:

- in its own directory, with **its own goose version table** (`goose.SetTableName`), run after
  the core's. It never numbers into the core sequence;
- only adding, never changing a core table: new tables referencing core ids, or new nullable
  columns. Renaming, dropping, or adding `NOT NULL` without a default on a core column breaks the
  generated code and later core migrations;
- read through its own queries and shown through `Register` routes and its own pages.

`ARCHITECTURE.md`'s extension contract gains both layers, and when to use which.

## Who sees a field

### User fields

`visibility`:
- `PUBLIC`: shown wherever the user is shown (lists, decisions, laudo signature);
- `PRIVATE`: the user and admins only. Use it for personal data: phone, documents. LGPD; book
  ch. 11.

A user can edit their own fields only when the definition says `self_editable`. The others are
set by an admin, as `job_title`'s capability neighbours are.

### Case fields: the bias question

Case fields are where contextual information goes: the crime, a suspect's name, the delegacia's
view. Examiners must not see that while they work blind (`analyses.BlindWork`,
`docs/access-profiles-plan.md`, book ch. 3). So `visibility` for a case field is:
- `CONTEXT` (**the default**): hidden from an examiner during blind work, shown to intelligence,
  coordinators, and the examiner once blindness ends;
- `TASK_RELEVANT`: needed to examine (e.g. "lift taken from glass", the capture device), shown to
  everyone who reads the case.

Defaulting to `CONTEXT` means a forgotten setting hides a field from the examiner rather than
biasing them.

## Examples (to test the model, not to ship)

- **Users**: unit (`SELECT`), professional class (`SELECT`), registration in a professional body
  (`TEXT`, `PUBLIC`, `in_report`), date certified in fingerprint / facial (`DATE`), phone
  extension (`TEXT`, `PRIVATE`).
- **Every case**: origin system's case number (`TEXT`, `TASK_RELEVANT`), priority (`SELECT`).
- **CRIMINAL**: police report (BO) and inquérito numbers (`TEXT`, `CONTEXT`), requesting
  delegacia (`SELECT`, `CONTEXT`).
- **CIVIL**: disaster event (`SELECT`), body or IML number (`TEXT`, `TASK_RELEVANT`).
- **FINGERPRINT**: lifting method (`SELECT`, `TASK_RELEVANT`), lifted by (`TEXT`).
- **FACIAL**: image source (CCTV, phone, social media: `SELECT`, `TASK_RELEVANT`), camera id.

Some of these overlap the occurrence layer, e.g. the BO number. Where a field belongs to the
occurrence, it should wait for occurrence fields rather than be copied onto every case.

## Reports

Fields with `in_report` join the laudo's data, built from the database at issue time
(`reports/data.go`). Each one is frozen in the snapshot **with its label as of issue**, so a later
relabel does not change an issued laudo. A user field with `in_report` (e.g. the professional
registration) goes into the signature block. A template names a field by key. A field the
template does not name is not printed.

## Custody ledger

`biometric_cases` and `users` are not among the chained tables (`docs/hash-chain-plan.md`), so
edits to `extra` are not chained. What reaches a laudo is fixed by its snapshot, and the snapshot
is chained through issuance. If an organization's field matters to custody (e.g. a seal number),
it should live in a chained table, or the open question below decides to chain case fields.

## Phases

1. **Schema and package.** A migration adding `extra` to `users` and `biometric_cases`, and
   `field_definitions`. Then `fields/` (types, `Validate`, merge), `cmd/fields`, and tests: key and
   type immutability, scope by case type and modality, required only on new writes.
2. **API.** `GET /api/fields?entity=&case_type=&modality=`; `extra` in the user and case JSON,
   filtered by visibility and blind work; merge-PATCH on users (admin and self) and cases (where
   the user may edit the case); `extra` on case creation and on request registration.
3. **Screens.** One `ExtraFields` component (form and read view per type, i18n labels) used in
   `Users`, `UserProfiles`, `CaseDetail` and the case and request creation forms.
4. **Reports.** `in_report` fields in the data and snapshot; the signature block.
5. **Docs.** The extension contract in `ARCHITECTURE.md`: both layers, the separate goose table,
   "add, never change".
6. **Later.** Filtering the case list by `SELECT` fields; fields on occurrences and requests; an
   admin screen for definitions; making `person.meta` merge, or replacing it with `extra`.

**trackid-sim** can ship a small `fields.yaml` (e.g. a DVI body number, a CCTV source) so the
synthetic world exercises the declared fields and the blind-work filter.

## Open

- **Definitions in a file, or editable in the app?** Proposed: file and command first.
- **Who may load definitions**: any admin, or only a super admin?
- **Should case fields be chained** in the custody ledger (adding `biometric_cases` updates to
  its covered tables)?
- **Which fields should the core own** rather than leave to organizations? A real column is
  better when every organization needs it, e.g. a professional registration for the laudo.
- **Validation in the database too** (a trigger checking `extra` against the definitions), so an
  import command that skips `Validate` cannot write bad data?
