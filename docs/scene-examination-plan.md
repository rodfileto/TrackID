# The scene: field notes offline, the scene examination and its laudo

Decided 2026-11-11. It continues `docs/scene-collection-plan.md`, whose "Later" left the scene
examination and its laudo unbuilt, and adds what an examiner needs at the scene itself: notes
taken with no signal, synced later.

## What is there now

- **Scene collection** (`collections`, migration 019): one form registers the occurrence, the
  fingerprint case, the collection (who, when, where, notes), its vestiges and seals, and the
  files. The request that formalizes the scene arrives later.
- **Marking traces** on an evidence image (`TraceMarkerModal` over `ImageViewer`, with minutiae
  marking for fingerprints) is on the case page only. A trace is `QUESTIONED` or `REFERENCE`
  (known material sent in for comparison).
- **`CRIME_SCENE_EXAMINATION`** and its report type exist in the schema but are not offered.
- **A laudo is laid out in Go** (`reports/template.go`), one layout for every deployment.
- **Nothing works offline**: no service worker, nothing kept in the browser.
- **A request's other numbers** are labelled rows (`case_request_references`: a task in the
  institute's case system), and an occurrence has its inquiry number
  (`occurrences.inquiry_number`). Both stay as they are.

## Decisions

1. **Field notes are a draft, kept in the browser first.** A page for a phone at the scene:
   the place (type, name, point from the device, address typed), three times (called out,
   examined, occurred), the team, other officers present, notes, and recorded audio notes.
   - It opens with no signal: a service worker keeps the application's files, and the notes are
     kept in the browser (IndexedDB) until synced.
   - Each note has an id made on the device (a UUID), and so has each audio. Syncing the same
     note twice changes nothing: the server keeps one row per id.
   - Nothing is geocoded at the scene. The address is found from the point when the note is
     synced (the census list, `docs/geocoding-plan.md`), and only if none was typed.
   - Signing in is asked for at sync, not to write: a session that ended in the field does not
     lose the notes.
2. **On the server a synced note is a draft too** (`scene_notes`, with `scene_note_audios`):
   its author's, listed for them, and for coordinators. It is not a case and holds no vestige.
3. **A note becomes a scene collection** through the existing form, which opens filled from it
   (place, times, team, notes). Registering marks the note converted and moves its audio notes
   to the collection.
4. **A collection says more.** `case_collections` gains the time the team was called out
   (`called_at`), other officers present (`other_officers`) and its team
   (`case_collection_members`: users besides the collector). Audio notes are files of the case
   with their collection as origin (`case_files.category = 'collection_note'`): they are the
   examiner's record, not evidence, and have no vestige.
5. **The scene examination is offered** on a fingerprint case with at least one collection. It
   takes the case's collections as its subject and answers the request that formalizes them.
   - It may hold pairs, as a comparison does: the scene laudo's comparison section. With none,
     it goes from examination straight to its report.
   - Fingerprint examiners and coordinators can open one, as they can a comparison.
6. **The analysis workspace gets the material.** For fingerprint analyses, the examiner sees the
   case's evidence images there and marks traces on them (the case page's tool), as questioned
   or reference. Reference documents (a card, an identity document sent in or brought by the
   examiner) are added there as evidence whose traces are `REFERENCE`, and pairs are made from
   the traces marked. Each pair's examination text goes to the laudo, as in a comparison.
7. **A deployment can lay out a report of its own.** `reports.RegisterLayout(reportType, fn)`
   replaces core's layout for one report type, with core's document builder exported for it.
   Core has no layout for the scene laudo: an institute's format is its own, so the scene
   examination's report is available only where a layout is registered. The report's data
   (`reports.Data`) gains the scene: collections with place, times, team and officers, vestiges
   with seals, the traces marked with their suitability, and the pairs with their results.
8. **The inquiry number stays on the occurrence, and a task number on the request.** The
   field-note form does not ask for either. The collection form's occurrence takes the inquiry
   number, and the request, when it arrives, takes its references.

## Schema (migration 027)

```sql
CREATE TABLE scene_notes (
    id BIGSERIAL PRIMARY KEY,
    client_id UUID NOT NULL UNIQUE,          -- made on the device: a second sync is the same row
    author_id BIGINT NOT NULL REFERENCES users(id),
    place_type TEXT, place_name TEXT,
    lon DOUBLE PRECISION, lat DOUBLE PRECISION,   -- the device's, or typed
    address TEXT, neighbourhood TEXT,
    geocode_source TEXT, geocode_flags TEXT[] NOT NULL DEFAULT '{}',
    called_at TIMESTAMPTZ, examined_at TIMESTAMPTZ, occurred_at TIMESTAMPTZ,
    team BIGINT[] NOT NULL DEFAULT '{}',     -- users
    other_officers TEXT, notes TEXT,
    written_at TIMESTAMPTZ NOT NULL,         -- on the device
    synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    collection_id BIGINT REFERENCES case_collections(id) ON DELETE SET NULL -- set when converted
);
CREATE TABLE scene_note_audios (
    id BIGSERIAL PRIMARY KEY,
    scene_note_id BIGINT NOT NULL REFERENCES scene_notes(id) ON DELETE CASCADE,
    client_id UUID NOT NULL UNIQUE,
    title TEXT, content_type TEXT NOT NULL, size_bytes BIGINT NOT NULL,
    sha256 TEXT NOT NULL, object_key TEXT NOT NULL, recorded_at TIMESTAMPTZ
);
ALTER TABLE case_collections ADD COLUMN called_at TIMESTAMPTZ, ADD COLUMN other_officers TEXT;
CREATE TABLE case_collection_members (collection_id, user_id, PRIMARY KEY (collection_id, user_id));
-- case_files.category also takes 'collection_note'.
```

## Phases

1. **Field notes on the server**: migration 027, `scenenotes` (sync, list, get, audio, discard),
   their routes and tests.
2. **Field notes on the device**: the service worker, the local store, the page (write, record,
   list, sync), and the collection form filled from a note; the collection's new fields.
3. **The scene examination**: offered, opened over the case's collections, with or without
   pairs; in the workspace, the material, marking, reference documents and pairs.
4. **Reports**: `RegisterLayout`, the builder exported, the scene in `reports.Data`.
5. **The institute's laudo** lives outside core (its own layout and labels).

## Status

- 2026-11-11: plan written. **Phase 1 done**: migration 027, `scenenotes`, `/api/scene-notes`
  (sync, list, get, discard, audio), a collection's call-out time, team, other officers and audio
  notes, and a note converted by `collections.Register` (`SceneNoteID`). `verify-storage` reads the
  audio notes too. Checked by `collections/scenenotes_db_test.go`.
- 2026-11-11: **Phase 2 done**: the service worker (`frontend/src/offline/sw.template.js`, written
  into each build by `vite.config.ts`), the device's store (`fieldNotesStore.ts`), the page
  (`/field-notes`, outside the session's guard), and the collection form filled from a note
  (`/collections/new?note=`), with the call-out time, team and other officers; the case page shows
  them and plays the audio notes. Checked in a headless browser on the built frontend: with the
  server stopped and the browser offline, the page opened, a note with a recorded audio was kept
  with the team list; when the server and the network came back it synced by itself, got its
  address from the census list, and was registered as a collection through the form.
- 2026-11-11: **Phases 3 and 4, backend**: the scene examination is offered (opened as a follow-up
  only on a case with a collection; a request may still ask for one first), may hold pairs, and with
  none goes from examination to its report. Migration 028 adds a third origin for material: the
  analysis that brought it in (`case_files.case_analysis_id`; a reference document has no vestige).
  The first examiner of a fingerprint examination lists and takes back pairs
  (`/analyses/:id/examiner-pairs`). Reports: `reports.RegisterLayout`, `reports.Document`, and
  `Data.Scene` (occurrence, collections and team, vestiges and seals); a scene examination's report
  lists every lift marked on the case. Core registers no scene layout: drafting one without a
  layout is refused (`ErrNoLayout`). Checked by `api/scene_examination_db_test.go`.

