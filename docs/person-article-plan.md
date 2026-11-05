# Person articles: the analytical layer, Wikipedia style

Planned and decided 2026-11-01 (see "Decided 2026-11-01" and "Also decided"), not yet built.
This is the first part of the intelligence and analytical layer. The person page shows what the
data says about a person. This plan adds what analysts **write** about them: one shared article
per person, in the style of a Wikipedia page, with `@` mentions of cases, occurrences, inquiries
(IPL), persons, clusters, requests and laudos.

Persons only for now. The model is generic enough that occurrences, inquiries or criminal groups
can get articles later without a new schema.

## What is there now

- **The person page** (`frontend/src/pages/PersonProfile.tsx`, route `/persons/:personRef`, by `persons.id`) shows
  data only, in stacked cards:
  - Person;
  - Identity: documents, registers, enrolled files;
  - Clusters, with their members;
  - Related cases.

  It is read from `person.GetProfile` (`person/profile.go`), which uses only the REVIEWED cluster
  layer and "asserts no link of its own".
- **Nothing written about a person is stored.**
  - `person.meta` is JSONB that importers fill. No screen writes it.
  - `case_requests.notes` is the only other free text, and it belongs to a request.
- **Access** (`docs/access-profiles-plan.md`):
  - the full profile is for intelligence only (`intelligence` group in `api/router.go`);
  - every read of a person goes to `access_log` (`LogAccess(..., accessPerson, "personId")`);
  - a user working blind anywhere loses intelligence rights until they submit.
- **Mention targets and their keys:**

  | Kind | Table | Key | Readable label |
  |---|---|---|---|
  | Case | `biometric_cases` | `case_id` (`2026.0001`, or an imported id) | case id + modality |
  | Occurrence | `occurrences` | `id` | BO `agency occurrence_number`, or TrackID `year.seq` |
  | Inquiry (IPL) | `occurrences.inquiry_number` | the number (text) | "IPL n" |
  | Person | `persons` | `person_id` | name on first register |
  | Cluster | `clusters` | `id` (+ `basis`) | "Cluster n" + modality |
  | Request | `case_requests` | `id` | case + `document_reference` |
  | Laudo | `analysis_reports` | `id` | type + `report_year/report_number` |

  An IPL is not a table. It is a field that one or more occurrences share, so a mention of it
  resolves to a set of occurrences.
- **No page exists for an occurrence or an inquiry.** The map (`/intelligence/map`) and the
  new-occurrence form are the only occurrence screens. Requests and laudos are shown inside the
  case page.
- **No rich-text editor in the frontend.** Reports are edited in WordCraft (a .docx editor), which
  does not fit a wiki with inline mentions.

## Decided 2026-11-01

1. **One shared article per person.** Any intelligence analyst in scope edits it. Every save is a
   new revision, kept forever, with its author, time and edit summary, as in Wikipedia's history.
2. **Mentionable in v1:** cases, occurrences (BO / TrackID number), IPL, persons, clusters,
   requests and laudos.
3. **Intelligence grading (Admiralty code, source A–F × information 1–6) comes later.** The model
   leaves room for it (decision 9 below).
4. **Backlinks are for intelligence only.** "Mentioned in the articles of X, Y" appears on a case,
   person or occurrence view only to intelligence users. Examiners and coordinators never see an
   article or a backlink, which keeps person context out of examination.

## Proposed model

### 1. The page: data and analysis, side by side but separate

The person page becomes a Wikipedia-style page with three tabs: **Article** (default), **Data**
(today's cards, unchanged) and **History**.

The Article tab:

```
┌──────────────────────────────────────────────┬───────────────────────┐
│ JOÃO DA SILVA                      [Edit]    │ ┌───────────────────┐ │
│ Last edited 2026-10-31 by asilva · 14 revs   │ │  enrolled photo   │ │
│                                              │ └───────────────────┘ │
│ Lead paragraph (the summary, first block)    │ Person id   P-000123  │
│                                              │ Born        1990-…    │
│ Contents                                     │ Documents   RG, CPF   │
│  1 Background                                │ Clusters    2 (F, D)  │
│  2 Known activity                            │ Cases       5         │
│  3 Associates                                │ Occurrences 3         │
│                                              │ First seen  2024-03   │
│ ## Background                                │ Last seen   2026-09   │
│ … was identified in @2026.0042 through       │                       │
│ @Cluster 311, the robbery in @BO 123/2026 …  │  (infobox: generated  │
│                                              │   from data, read-    │
│ ## Associates                                │   only)               │
│ … acts with @Maria Souza …                   │                       │
│                                              │ Mentioned in          │
│ ─ What links here: 2 articles                │  · Maria Souza        │
└──────────────────────────────────────────────┴───────────────────────┘
```

- **The infobox is generated, never typed.** It comes from `GetProfile` plus the occurrences of
  the person's cases: photo (first enrolled facial file), names, birth date, documents, clusters
  per modality, cases, occurrences, first and last occurrence dates. The article can't contradict
  the data by editing it.
- **The text is labelled as analysis:** "Intelligence analysis — the analysts' assessment, not a
  forensic finding." This keeps `GetProfile`'s rule (data asserts no link of its own) and makes
  plain that the article is opinion.
- **Contents** are built from the article's headings (h2, h3).
- **An empty article** shows the infobox plus "No article yet — Write one" for users who can edit.

### 2. The editor: TipTap, with a constrained schema

- **[TipTap](https://tiptap.dev) 3** (ProseMirror underneath, MIT), with only free extensions:
  - `@tiptap/react` and `@tiptap/starter-kit`;
  - `@tiptap/extension-mention` and `@tiptap/suggestion`, for the `@` menu;
  - `@tiptap/extension-link`.

  Not used: TipTap's paid Pro and Cloud extensions (collaboration, comments, AI). Lexical (Meta,
  MIT) is the alternative. TipTap's mention extension is closer to what is wanted out of the box.
- **Allowed nodes:**
  - paragraph and headings (h2, h3; the page title is the h1);
  - bullet and numbered lists, blockquote, hard break;
  - bold, italic, link (external links only through `https:`);
  - `mention`.

  No images, tables or raw HTML in v1. The server checks every saved document against this list
  and rejects anything else, so a crafted request can't store markup the viewer would render.
- **Stored format:**
  - the canonical body is **ProseMirror JSON**;
  - the server also keeps **plain text** derived from it, for full-text search and for diffs.
- **Read mode** renders the same JSON with the editor set read-only, so viewing and editing show
  exactly the same thing.

### 3. Mentions, Instagram style

- **Typing `@`** opens a dropdown that follows the caret. It searches every kind at once and
  groups the results by kind, with an icon for each: case, occurrence, IPL, person, cluster,
  request, laudo.
  - Up to 5 results per kind. Arrow keys move, Enter or Tab inserts, Esc closes.
  - **Optional prefixes** narrow the search: `@caso`, `@bo`, `@ipl`, `@pessoa`, `@cluster`,
    `@oficio`, `@laudo` (localized; `@case`, `@person` … in English). `@ipl 4512` searches only
    inquiries.
  - What each kind matches:
    - a case: id, or words in its description;
    - an occurrence: BO number, TrackID number, inquiry number or description;
    - a person: name (the trigram index already exists), person id or document number;
    - a cluster: id;
    - a request: `document_reference` or `authority`;
    - a laudo: its number.
- **A mention is an inline node** `mention { kind, ref, label }`:
  - `ref` is the stable key in the table above. Clusters use `id`, occurrences, requests and
    laudos use their row id, cases use `case_id`, IPLs the number.
  - `label` is a snapshot taken when the mention was inserted.
- **When the article is read, every mention is resolved again:**
  - **live label:** a renamed person shows their current name;
  - **removed target:** shown struck through, with the snapshot label and "removed";
  - **merged cluster:** followed through `cluster_merges` to the surviving cluster, with a note
    "now Cluster m";
  - **target outside the reader's scope** (once organizations and units exist,
    `docs/organizations-plan.md`): shown as "restricted reference", with no label and no link.
    The snapshot is never shown either, so an article can't leak another unit's case number.
- **Each chip links to its target:**
  - case → `/cases/:caseId`;
  - person → `/persons/:personRef` (`persons.id`, so the `person_id` stays out of the browser's history);
  - cluster → `/clusters?cluster=:id`;
  - request and laudo → the case page, on its requests or report section;
  - occurrence → a new **occurrence page**, `/intelligence/occurrences/:id`;
  - IPL → a new **inquiry page**, `/intelligence/inquiries/:number`, listing its occurrences
    and their cases.

  Hovering a chip shows a small card: type, date, status and, for a case, its modality and how
  many traces it has.
- **Laudos:** only issued ones can be mentioned. A draft has no number and may still change.

### 4. Schema (migration `010_articles.sql`)

Generic by subject, with only `PERSON` allowed for now:

```sql
CREATE TABLE articles (
    id BIGSERIAL PRIMARY KEY,
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('PERSON')),
    subject_ref TEXT NOT NULL,              -- person.person_id
    current_revision_id BIGINT,             -- FK added after article_revisions
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT articles_subject_key UNIQUE (subject_kind, subject_ref)
);

-- The record of each revision: who, when, why. Append-only: a BEFORE UPDATE OR DELETE trigger
-- raises (as hash-chain-plan decision 3).
CREATE TABLE article_revisions (
    id BIGSERIAL PRIMARY KEY,
    article_id BIGINT NOT NULL REFERENCES articles(id),
    revision_number INTEGER NOT NULL,
    parent_revision_id BIGINT REFERENCES article_revisions(id),
    body_sha256 BYTEA NOT NULL,             -- hash of the body as saved; survives a redaction
    body_chars INTEGER NOT NULL,            -- size, for the history's +/- column
    edit_summary TEXT,
    author_id BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT article_revisions_number_key UNIQUE (article_id, revision_number)
);

-- The text of each revision, kept apart from its record so that a later redaction (decision 8)
-- can remove the text and leave the record. In v1 a trigger forbids UPDATE and DELETE here too.
CREATE TABLE article_revision_bodies (
    revision_id BIGINT PRIMARY KEY REFERENCES article_revisions(id),
    body JSONB NOT NULL,                    -- ProseMirror JSON, validated by the server
    body_text TEXT NOT NULL                 -- plain text, for search and diff
);
CREATE INDEX article_revision_bodies_text_idx ON article_revision_bodies
    USING GIN (to_tsvector('simple', body_text));

-- The mentions of each article's current revision, rewritten on every save: what backlinks read.
CREATE TABLE article_mentions (
    article_id BIGINT NOT NULL REFERENCES articles(id),
    kind TEXT NOT NULL CHECK (kind IN ('CASE','OCCURRENCE','INQUIRY','PERSON','CLUSTER','REQUEST','REPORT')),
    ref TEXT NOT NULL,
    mention_count INTEGER NOT NULL DEFAULT 1,  -- times mentioned in the article
    PRIMARY KEY (article_id, kind, ref)
);
CREATE INDEX article_mentions_target_idx ON article_mentions (kind, ref);
```

- `subject_ref` is the text key rather than a foreign key, so other subject kinds fit later. The
  save checks that the person exists.
- **No foreign keys from mentions to their targets:** a deleted case must not take an article's
  history with it. The text keeps the snapshot label, and reading shows the target as removed.
- Once organizations exist, `articles` gains its organization (and unit), as the other tables do.

### 5. Saving and conflicts

- **Saves are optimistic:** a save carries the revision it started from.
  - If that revision is no longer current, the server answers **409** with the newer revision.
  - The editor then shows the two side by side, the analyst merges by hand and saves again.
  - There is no real-time co-editing in v1. Articles are edited rarely, so the merge is cheap.
- **A save, in one transaction:**
  1. check the body against the schema (§2);
  2. check that every mention exists and is within the author's scope;
  3. take the plain text;
  4. insert the revision (its record and its body, with the body's SHA-256 and size) and move
     `current_revision_id`;
  5. rewrite `article_mentions`.
- **Edit summary:** optional, one line, shown in the history.
- **Restoring an old revision** means saving its body as a new revision ("restored revision n").
  Nothing is ever deleted.

### 6. History and diff

- **The History tab:**
  - lists the revisions: number, author, time, summary, and size change (+/− characters);
  - opens any old revision read-only;
  - compares two revisions.
- **The diff** runs on `body_text`, by paragraph and then by word, with
  [jsdiff](https://github.com/kpdecker/jsdiff) (`diff`, BSD-3). It is shown inline, insertions
  green and deletions red. A mention appears in the text as `@label` so its changes show too.

### 7. Access and audit

| | Examiner | Intelligence | Coordinator | Admin |
|---|---|---|---|---|
| Read a person's article, history, backlinks | no | yes (within scope) | no | no |
| Write a person's article | no | yes (within scope) | no | no |
| `@` mention search | no | yes (scope-filtered) | no | no |
| Occurrence and inquiry pages | no | yes | no | no |

- **Blindness suspends all of it,** as for the rest of intelligence.
- **Every article read and every history read goes to `access_log`** as a read of the person
  (kind `PERSON`, subject the person id); the route tells an article read apart. Reading an old
  revision is a read too.
- **Writes need no extra log:** the revision itself records who wrote what and when.
- **An article never reaches a laudo, a report snapshot or a request.** Nothing in
  `reports/` reads these tables. A test enforces it (no query in `reports/` names them).
- **Custody ledger** (`docs/hash-chain-plan.md`): articles stay out of it for now (decision 5).

### 8. Backlinks (intelligence only)

- **Case page:** a "Mentioned in articles" panel, for intelligence users only. Its query asks for
  mentions of the case, its requests, its laudos and its occurrence.
- **Person page:** "What links here" on the Article tab: the other persons' articles that mention
  this one.
- **Occurrence and inquiry pages:** the articles that mention them.
- **Mentions between persons form a graph.** It could later become a layer on the Clusters graph:
  analyst-asserted links, drawn apart from the biometric ones. Later, not v1.

### 9. Room for grading (later)

- **What it will be:** a `citation` inline node `{ mention, source_grade A–F, info_grade 1–6,
  note }`, numbered as footnotes and listed in a "Sources" section at the end, as Wikipedia's
  references are.
- **Why v1 needs nothing for it:** the node can be added to the allowed schema later, and old
  revisions stay valid.
- **What v1 does now:** mentions already act as inline sources, so a statement can name the case
  or laudo it rests on.

### 10. Packages and files

- **Backend:**
  - `articles/`, a public core package:
    - `Get(subject)` returns the article, its current revision and the resolved mentions;
    - `Save(subject, baseRevision, body, summary, author)`;
    - `History`, `Revision`, `Backlinks(kind, ref)`;
    - `Validate(body)`, the schema allowlist;
    - `ExtractMentions(body)` and `PlainText(body)`.
  - `articles/mentions.go`: `SearchTargets(q, kinds, scope)` and `Resolve(mentions, scope)`.
  - Queries in `db/queries/articles.sql`, regenerated with sqlc.
  - `api/articles_handler.go`, with routes under the `intelligence` group:

    ```
    GET  /api/persons/:personId/article
    PUT  /api/persons/:personId/article                  {baseRevisionId, body, summary}
    GET  /api/persons/:personId/article/history
    GET  /api/persons/:personId/article/revisions/:n
    GET  /api/mentions/search?q=&kinds=
    GET  /api/backlinks?kind=&ref=
    GET  /api/occurrences/:id                             (occurrence page)
    GET  /api/inquiries/:number                           (inquiry page)
    ```

  - `api/access_db_test.go` gains every new route, for every profile, the blind user included.
- **Frontend:**
  - `components/articles/`:
    - `ArticleView` and `ArticleEditor`, the TipTap setup with a shared schema;
    - `MentionMenu`, the grouped dropdown;
    - `MentionChip`, with its hover card;
    - `ArticleToc`, `ArticleHistory`, `RevisionDiff`;
    - `PersonInfobox`.
  - `PersonProfile.tsx` is restructured into the tabs. Today's cards move unchanged into the
    Data tab.
  - New pages `OccurrenceDetail.tsx` and `InquiryDetail.tsx`.
  - `services/articles.ts`, and i18n keys in `en.json` and `pt-BR.json`.

## Phases

1. **Schema and backend:**
   - the migration, `articles/` (validate, extract, save, history, mentions search and resolve)
     and the handlers;
   - unit tests for validation, which must reject unknown nodes and `javascript:` links;
   - DB tests for saves and conflicts;
   - the access-table test.
2. **Reading and editing:**
   - TipTap with the `@` menu;
   - the Article tab with the infobox and contents;
   - the person page restructured into tabs.
3. **History, diff and backlinks:** the History tab, plus the backlinks panels on the case and
   person pages.
4. **Mention targets:** the occurrence and inquiry pages, so every chip leads somewhere.

**Later:**
- revision deletion (decision 8);
- grading and citations (§9);
- a talk page per article;
- articles for occurrences, inquiries and criminal groups (another `subject_kind`);
- the person-to-person mention graph;
- LLM-drafted summaries through `assist/`, offered as a draft for the analyst and never saved
  without their edit.

## Also decided 2026-11-01

5. **Not in the custody ledger for now.** `article_revisions` stays out of the hash chain's
   covered tables. It is append-only by trigger, but has no tamper evidence beyond that. This can
   be revisited once the ledger exists.
6. **Every intelligence analyst in scope may write.** There is no editor flag. The history is the
   check.
7. **trackid-sim writes no articles for now.** The layer is demonstrated by hand.
8. **Redaction: none in v1, built so it can be added.** Removing a sentence from the article
   only adds a revision; the old one still shows the text in History. That is accepted for v1.
   - **Built now:** a revision's text (`article_revision_bodies`) is stored apart from its record
     (`article_revisions`: author, time, summary, size, the body's SHA-256). The text can later be
     removed without touching the record. Both tables are append-only in v1.
   - **Follow-up, "revision deletion" (Wikipedia's term):**
     - a new capability flag, `redacts_articles`, granted by an admin like the other flags;
     - a user who holds it hides one revision's text with a **required reason**;
     - the text is **destroyed**: its row in `article_revision_bodies` is deleted through a
       single function that is the only path allowed past the trigger;
     - the revision stays in History with its author, time, size and hash, marked "content
       redacted on [date] by [user]: [reason]";
     - an append-only `article_redactions` table (`revision_id`, `redacted_by`, `reason`,
       `redacted_at`) records the act, so redaction can't quietly rewrite history;
     - diffs skip a redacted revision and compare its neighbours;
     - the current revision can't be redacted; the article is edited first, and then the old
       revision is redacted.
   - **Uses:** an accusation a later laudo disproved, a witness's or victim's name, sensitive
     data or sealed material pasted in by mistake, a court order.
   - **Not chosen:** keeping the hidden text readable to the flag's holders (Wikipedia's
     "oversight"). It would keep the very data the redaction exists to remove.

## Open

None at present.

## As built

**Phase 1, 2026-11-01** (schema and backend):

- `db/migrations/010_articles.sql`: the four tables of §4, append-only triggers on both revision
  tables (`UPDATE`, `DELETE` and `TRUNCATE` refused).
- `articles/`:
  - `ParseBody` checks a body and returns its canonical form. Unknown node and mark types are
    refused. Attributes the schema does not know (the editor's defaults: `textAlign`, a link's
    `target` and `rel`, TipTap's `mentionSuggestionChar`) are **dropped**, not refused, so a
    stock TipTap document saves. Mark order is sorted, so equal texts hash equal.
  - `Get`, `Save`, `History`, `GetRevision`, `Backlinks`, `CaseTargets`; `Search` and
    `Resolve` for mentions.
  - A save that changes nothing writes no revision. A new mention must name a target that
    exists; mentions the current revision already holds may stay after their target is gone.
- **Search limits**, for population-scale tables: persons are found by a name on any register
  (trigram index) or their exact person id, never with an empty term; document numbers are not
  searched (no index on them alone). Clusters are found by the leading digits of their id.
- **Requests can belong to several cases** (`case_request_cases`), so a request target carries
  all of its case ids.
- **Routes** (intelligence only, reads logged as above): `GET/PUT
  /api/persons/:personId/article`, `…/article/history`, `…/article/revisions/:number`,
  `…/article/backlinks`, `GET /api/cases/:caseId/article-backlinks`, `GET
  /api/mentions/search?q=&kinds=&limit=`. A stale save answers 409 with the article as it now is;
  a refused body or unknown mentions answer 422 (`missing` lists them).
- **Tests:** body schema (`articles/body_test.go`), saves, conflicts, mentions, append-only and
  search against Postgres (`articles/articles_db_test.go`), the routes
  (`api/articles_handler_test.go`), the access table, and a check that no forensic package
  (reports, matcher, clusters, examination, analyses, person views) reads articles
  (`articles/isolation_test.go`).

**Drafts from the records, 2026-11-01** (asked for after phase 2; not in the original plan):

- An empty article offers **"Generate draft from the records"** beside "Start blank".
  `articles.PersonTemplate` writes the draft. **Intelligence works with uncertainty**, so the
  draft does not stop at what examiners confirmed (decided 2026-11-01, generator `person-v2`):
  - every case link carries its **certainty**, shown as one subsection each:
    - **confirmed** — in the REVIEWED layer;
    - **not reviewed** — the matcher's: a pending candidate (with its score), or reached only
      through the AUTOMATIC cluster layer (named);
    - **uncertain** — examined, but disputed or inconclusive;
    - **excluded** — examiners ruled it out (kept: it is information too).

    Direct pairs come from `cases.ListPersonTraceDecisions`, the person side of the case
    page's identity decisions, with the same statuses.
  - a lead paragraph (name, person id, birth date, confirmed links and those left open);
  - Identification (names, flagging several as possible aliases; documents; birth; parents);
  - Biometric record: the reviewed clusters, then the automatic clusters that hold more, and
    any other person's enrolment either layer groups with this one, flagged as a possible
    second identity;
  - Cases and occurrences, by certainty, with each case's occurrence and date, IPL, requests and
    issued laudos (not for excluded ones);
  - Associates: persons the AUTOMATIC layer links to the same (non-excluded) cases, each shared
    case marked confirmed (both links reviewed) or not reviewed;
  - Assessment.

  Every fact is a mention. What only the analyst can say is an italic, bracketed prompt, and a
  legend explains the certainty levels. Wording in Portuguese or English, after the interface
  language.
- **The infobox counts links by certainty** too (`articles.PersonLinkCounts`, route
  `GET /api/persons/:personId/link-counts`, intelligence only, logged as a read of the person):
  cases confirmed, not reviewed, uncertain and excluded; reviewed and automatic clusters; other
  identities and associates, confirmed or not. Unreviewed counts are highlighted, with a note
  that they are leads, not findings. The counts and the draft read the same gathered links
  (`articles/links.go`). The Data tab still shows the REVIEWED layer only, as the forensic person
  view does.
- **Logged:** every draft generated is written to `article_templates`
  (`db/migrations/010_articles.sql`), saved or not: subject, generator version
  (`person-v1`), language, the draft and its hash, who generated it and when. Append-only like
  the revisions. Generating also counts as a read of the person in `access_log`.
- **Traced to the revision:** a save names its draft (`article_revisions.template_id`); the
  article header and the history show "started from a draft generated from the records".
- Route: `POST /api/persons/:personId/article/template` (`{"language": "pt-BR"}`), intelligence
  only.
