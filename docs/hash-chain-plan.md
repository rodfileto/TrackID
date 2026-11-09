# Forensic hash chain

Planned 2026-10-29. TrackID keeps the SHA-256 of every file and several append-only logs, but
nothing proves afterwards that a record was not changed, removed or back-dated. This plan adds a
**custody ledger**: one hash chain over the events that matter to custody and to the laudo, sealed
in order, anchored outside the database by signed checkpoints and by the laudos themselves, and
checked by a verifier anyone can run.

## What is there now

- **Files carry their SHA-256.** `case_files.hash_id` and `identity_files` (uploads in
  `cases/`, `identity/upload.go`), video (`video/video.go`), the image lab's original and output
  (`imageproc/decode.go`). Reports cite these hashes (`reports/data.go`, `reports/template.go`),
  and an issued report keeps its .docx and PDF as `case_files` with their hashes.
- **Append-only by convention only.** `biometric_decisions`,
  `analysis_assignments`, `cluster_merges` and `access_log` say "never updated or deleted" in
  their migration comments, but Postgres does not enforce it: any role with write access can
  `UPDATE` or `DELETE` them, and nothing would show it.
- **Some custody records change in place.**
  - `UpsertCaseFile` overwrites a file's metadata on conflict.
  - `DeleteCaseFile` erases an evidence file with no trace (`cases/detail.go`).
- **The access log exists** (`001_foundation.sql`, `api/auth_middleware.go`, see
  `docs/access-profiles-plan.md`). It records who read a person or a case they do not work on.
- **Organizations write into the core tables directly** (the extension contract in
  `docs/ARCHITECTURE.md`), so anything done only in the API would miss their imports.

## Decisions

Decided 2026-10-29: the chain covers **custody of material, decisions, report issuance and the
access log**. It is **one global ledger**, and it is anchored by **signed checkpoints plus the
laudo**. Fully local: no outside timestamp authority.

1. **One ledger table, written by the database.** Every insert, update or delete on a covered
   table appends one entry, through an `AFTER ... FOR EACH ROW` trigger. Because the trigger is
   in Postgres, the API, the worker, the `cmd/` tools and an organization's import commands are
   all covered. None of them can skip it.
2. **Covered tables** (first cut; Phase 1 confirms each against the code):

   | Group | Tables | Operations |
   |---|---|---|
   | Custody of material | `case_requests`, `case_request_cases`, `case_files`, `identity_files`, `case_evidences`, `case_videos`, `case_codifications` | insert, update, delete |
   | Decisions | `biometric_decisions`, `analysis_assignments`, `cluster_merges`, `match_thresholds` | insert |
   | Report issuance | `analysis_reports` (status changes), `analysis_report_versions`, `report_deliveries` | insert, update |
   | Access | `access_log` | insert |

   Derived data is not chained, because it can be recomputed from what is: embeddings,
   templates, clusters and their members. `match_thresholds` is in, because a threshold change
   alters what the system proposes.
3. **Append-only enforced.** The decision tables, `access_log`, `analysis_report_versions` and
   the ledger itself get a `BEFORE UPDATE OR DELETE` trigger that raises. A deleted evidence file
   stays in the ledger as a `DELETE` entry with its hash: the file goes, the record of it does
   not.
4. **Two steps: record, then seal.** A trigger that hashed the chain directly would have to hold
   one global lock until its transaction commits. A batch of SYSTEM decisions (1,691 in the demo
   world) would then stall every access-log write and every upload. Instead:
   - the trigger writes the event into `custody_pending`, in the same transaction as the change.
     No lock is taken, and an event whose transaction rolls back never exists;
   - a **sealer** in the worker (periodic, every few seconds, and on demand) moves pending events
     into `custody_ledger` in order, in short transactions under one advisory lock. It assigns
     `seq` and chains the hashes.
   
   The ledger order is seal order. Each entry also keeps the time of the event. The cost is a
   window of seconds, between commit and seal, in which an event is not yet chained. The verifier
   reports pending events older than a limit.
5. **Entry layout and hash.**
   - `custody_ledger`: `seq`, `occurred_at`, `sealed_at`, `source_table`, `source_id`, `op`
     (`INSERT`, `UPDATE`, `DELETE`, `SNAPSHOT`, `GENESIS`), `case_id` (when the row belongs to
     one), `actor` (username or system source when the row has one), `payload`, `payload_hash`,
     `prev_hash`, `entry_hash`.
   - `payload` is the row as written, `to_jsonb(NEW)::text` (or `OLD` for a delete), stored
     **as text**. The verifier hashes the stored bytes, so it never has to reproduce Postgres's
     JSON rendering.
   - `entry_hash = SHA-256(prev_hash ‖ seq ‖ occurred_at ‖ source_table ‖ source_id ‖ op ‖
     payload_hash)`. Each field is length-prefixed under a version tag
     (`trackid-custody-v1`). The first entry's `prev_hash` is 32 zero bytes.
   - The hash covers `payload_hash`, not `payload`. A payload can later be **redacted**
     (set to NULL) without breaking the chain, if a legal basis ever requires it. The fact that
     the event existed, and its hash, remain.
   - The layout is defined once, in a SQL function used by the sealer and in a Go package
     (`custody/`) used by the verifier. A test checks that the two agree.
6. **Genesis.** The first seal writes a `GENESIS` entry, then one `SNAPSHOT` entry per existing
   row of every covered table. History before that date is attested only as "it existed at
   genesis", and the verifier says so.
7. **Anchoring.**
   - **Signed checkpoints.** The worker signs the head, `(seq, entry_hash, time)`, with an
     Ed25519 key it reads from a file (`CUSTODY_SIGNING_KEY`), never from the database, hourly
     and at every report issue. It writes the checkpoint to `custody_checkpoints` and appends it
     to a file outside the database volume (`CUSTODY_CHECKPOINT_LOG`). The public key is
     published by the institution.
   - **The laudo.** Issuing seals first, then prints the head into the report: "Registro de
     custódia: posição N, hash …". Every issued laudo becomes an outside witness of the chain up
     to N. That chain covers the case's material and decisions. The issue itself is chained right
     after, because a document cannot contain its own hash.
   - Honest limit: someone who holds both the database and the signing key could rewrite the
     chain and sign it again. They could not alter the laudos already delivered, or copies of the
     checkpoint log kept elsewhere. Where the key and those copies live is the institution's
     decision (see Open).
8. **Verifier.** `cmd/custody-verify` checks:
   - the chain from genesis (each `prev_hash` and `entry_hash`);
   - each checkpoint's signature against the public key, and that its head is on the chain;
   - that the current state of each covered row matches its latest ledger payload, which catches
     an edit made with the triggers disabled;
   - optionally (`-objects`), that every file in MinIO still has the SHA-256 the ledger records;
   - a laudo's citation: `-at N -head <hash>`.

   It exits non-zero on any failure and prints a report in Portuguese that an auditor can attach.

## Phases

**Phase 0: schema and hash.** The migration adds `custody_pending`, `custody_ledger`,
`custody_checkpoints`, the hash function and the append-only triggers. The `custody/` Go package
mirrors the hash, with a test that SQL and Go agree on the same entries.

**Phase 1: recording and sealing.**
- Add the covered-table triggers.
- Add the sealer as a periodic Asynq task, and as a function the report issue calls.
- Write genesis and the snapshot.
- Database tests (the existing per-test databases):
  - every covered write appears once;
  - a rolled-back write never does;
  - concurrent writers yield one unbroken chain;
  - update and delete on the append-only tables fail.

**Phase 2: verifier.** Build `cmd/custody-verify`. Add tests that tamper with the database
(an edited payload, a deleted entry, a row changed with the triggers disabled, a swapped object
in MinIO) and expect the verifier to name each one. CI runs them, as it runs the other database
tests.

**Phase 3: anchoring.** Add checkpoint signing and the external log. Print the head in the laudo
and the lead report (`reports/`), and add `-at/-head` verification.

**Phase 4: operation.**
- An admin page with chain status: last seal, last checkpoint, pending count, last verification.
- The cost under load, measured with `trackid-sim stress`: the burst and the class, with and
  without the triggers.
- A section in `docs/ARCHITECTURE.md` for deployers: the key, the log copies, and running the
  verifier on a schedule.

## Open

- **Who holds the signing key, and where the checkpoint copies go.** For example, a separate
  host, the corregedoria, or a write-once volume. This is the institution's decision, not the
  code's.
- **`users`**: should changes of role and of `is_active` be chained (without `password_hash`)?
  Who could act is part of custody, but it was not in the chosen scope.
- **Case deletion.** With `biometric_decisions` append-only, deleting a case that has decisions fails. Decide whether
  cases are ever deleted, or only closed.
- **Retention and redaction.** Payloads hold personal data (usernames, file names, decisions).
  Check with the institution's data protection officer before deciding when, if ever, a payload
  is redacted (Decisions, 5).
