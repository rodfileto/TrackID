# Access profiles: examiner, intelligence, admin

Decided and built 2026-10-19 (see "As built" at the end). Who may see a person's history, the biometric cases and the
intelligence pages, separated from who runs the application.

## Why

Today any signed-in user can search persons, open a full person page (identity, clusters, every
linked case), browse every cluster and the cluster graph, search persons by face and open the
occurrence map; only distribution and user management are admin-only, and an admin sees every case.
That mixes three jobs:

- **Examining**: deciding pairs in the analyses one is given. Contextual information about the
  person or the case can bias the decision (book ch. 3), so an examiner should see as little as
  their work needs.
- **Intelligence**: linking people, cases and occurrences (book part II). It needs everything,
  read-only, and its reads are the ones privacy law and audit care about (book ch. 11).
- **Running the application**: accounts, configuration. It needs no case or person content.

## Model

`users` already separates `account_role` (SUPER_ADMIN, ADMIN, USER: account management only) from
capability flags (`examines_facial`, `examines_fingerprint`; migration 016). Intelligence is
another capability flag, not a role, so one person can examine and analyse:

- migration 020: `users.intelligence_analyst` and `users.coordinator`, `BOOLEAN NOT NULL DEFAULT
  false`; existing admins get `coordinator`, so distribution keeps working after the switch;
- `auth.Access` / `auth.UserProfile` gain `IntelligenceAnalyst`, with a `CanSeeIntelligence()` check;
- the admin user screen gets a checkbox; `CheckAccountChange` lets an admin grant it to users and a
  super admin to admins, as for the other flags;
- `/me` returns it, and the frontend's current-user context exposes it next to `isAdmin`.

**Coordination** is a third flag, `coordinator`: distributing work, reassigning steps and opening
comparisons on chosen pairs move from admins to it. Not intelligence: choosing which pairs to
compare is case management, and keeping it away from the people who know person histories keeps
that context out of the examination. A coordinator sees the distribution screens and, to open a
comparison, the case's own faces and the matcher's candidates (as the first examiner does), not
person histories.

## Access

Each column is one flag; a user with several gets the union, except that blindness (below) cuts
across all of them.

| Route / page | Examiner (plain user) | Intelligence | Coordinator | Admin (no flags) |
|---|---|---|---|---|
| `persons/search` (by name) | yes, results show identity only | yes | no | no |
| `persons/:id/identity`, identity file download | yes | yes | no | no |
| `persons/:id` (full profile), `/clusters`, `/cases` | no (403); the page shows the identity section only | yes | no | no |
| `persons/search-by-face` | no: it returns cases and clusters | yes | no | no |
| `clusters`, `clusters/graph` (Clusters page) | no | yes | no | no |
| `occurrences/*`, Intelligence menu (map, new occurrence) | no | yes | no | no |
| `cases` list, `cases/:caseId/*` read routes | cases where they hold an analysis | every case | every case, for distribution | no |
| `cases/:caseId/*` write routes (evidence, traces, points) | cases where they hold an analysis | cases where they hold an analysis | no | no |
| `cases/:caseId/occurrence` (link) | no | yes | no | no |
| Deciding pairs | only their step, per `analyses.CanWork` | only if they also examine | only if they also examine | only if they also examine |
| `admin/users` | no | no | no | yes |
| `admin/distribution`, open comparison, reassign | no | no | yes | no |

**Admin** is application management only: an admin with no other flag sees no case or person
content; one who needs it also gets the flag for it.

**Intelligence** reads everything and links occurrences to cases (intelligence work), but adds or
edits evidence only where it holds an analysis, as an examiner does.

**Coordinator** reading every case is the one place case content reaches someone who neither
examines nor analyses it; it is what opening comparisons needs. Its reads go to the access log
(below) like intelligence's.

### Blindness overrides every profile

While a user works an analysis blind (`analyses.BlindWork`: the REVIEWER at VERIFICATION, the
INCONSISTENCE examiner at INCONSISTENCE), `distribution.BlindIn` restricts them to that workspace
on the case, whatever their flags. Two changes follow:

- `CaseAccess` stops exempting admins from the blind restriction (today it does);
- the person, cluster, occurrence and face-search routes also refuse a user who is blind *anywhere*
  right now, not only on that case: an analyst verifying blind could otherwise look the reference
  person up in another tab. Simplest rule: a blind user's intelligence rights are suspended until
  they submit, with a clear message in the UI.

## Audit

An append-only `access_log` (user, time, route kind, person id or case id) written by the person
routes (full profile, clusters, cases, face search) and by the case routes when the reader holds
no analysis there (that is, intelligence reads). It answers "who looked at this person and when"
(LGPD, book ch. 11). No UI at first; an admin or super admin query can come later.

## Frontend

- Sidebar: Clusters, the Intelligence menu, and person search by face only with
  `intelligenceAnalyst`; Distribution only with `coordinator`; user management only for admins.
- Person page: without intelligence, render the identity section only and do not call the
  profile, clusters or cases routes (they would answer 403).
- Admin user screen: the new checkbox(es).
- Routes guarded on the client too, so a direct URL shows a "no access" page rather than an error.

## Tests

- `auth`: the new flag, and `CheckAccountChange` for granting it.
- An `api` test (with the throwaway database) walking each row of the table for a plain examiner,
  an analyst, an admin with no flags, and a blind analyst.
- The existing decision-flow test: an admin holding a blind step is restricted like anyone else.

## trackid-sim

`trackid-sim users` gives the instructor's account admin + intelligence + coordinator; the class's examiners and the planting account are plain examiners, under codes
(E01...), so nobody in the class sees person histories or who planted what.

## Order

1. Migration, `auth` flags (`intelligence_analyst`, `coordinator`), `/me`, admin checkboxes.
2. Route guards and the blind override; the access table as a test.
3. Frontend: sidebar, person page, route guards.
4. Coordinator flag: distribution and comparisons move from admin to it.
5. Access log.

## As built (2026-10-19)

- Migration 020 (flags; current admins became coordinators) and 021 (`access_log`).
  `distribution.Rights` decides case access; `RequireIntelligence`, `RequireCoordinator` and
  `RequirePersonIdentity` guard the other routes; `api/access_db_test.go` walks the table above
  through the router, the access log included.
- Blindness suspends coordination as well as intelligence (a coordinator's routes list examiners
  and open a case's analyses), and suspends person search and identity for examiners too.
- The coordination routes keep their `/api/admin/...` paths; only the guard changed.
- Person search sends `caseCounts` to analysts only.
- Reading a case's linked occurrence stays part of reading the case; finding, linking,
  unlinking and registering occurrences are intelligence.
- There is no user-management screen in the frontend (accounts are managed through
  `/api/admin/users` and `cmd/create-superuser`, now with `-intelligence` and `-coordinator`), so
  the admin checkboxes wait for that screen.
