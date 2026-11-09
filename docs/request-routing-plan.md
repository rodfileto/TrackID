# Routing requests to tasks

Decided 2026-10-28. It builds on "Request first" (`docs/facial-reports-plan.md`): the flow is
**occurrence → request → case**. The request is an outside document (an ofício), registered in
TrackID by a coordinator. Registering it routes the work: to new or existing cases, and in each
case to the **task** the request asks for. A case therefore arrives with a specific request and
a specific task.

## What is there now

- Registering a request finds or creates its occurrence, then opens or joins one case per exam
  asked (`requests.Register`). The coordinator picks only each case's **modality**, facial or
  fingerprint.
- Every facial case gets a `FACIAL_REVIEW` automatically (`analyses.OpenAutomatic`, called when a
  case is created or ingested), whatever the request asked.
- A `FACIAL_COMPARISON` is opened later by a coordinator, on chosen pairs.
- Fingerprint cases have automatic matching and the decision rounds, but no analysis types, no
  worksheet and no laudo.
- An analysis already records the request it answers (`case_analyses.case_request_id`).

## Decisions

1. **The coordinator always chooses the task.** For each case a request opens or joins, the
   coordinator names the task asked. Routing opens one analysis of that type in the case,
   answering the request. Nothing is opened automatically any more: `OpenAutomatic` is removed,
   and a case created or ingested without a request has no analysis until a coordinator opens
   one.
2. **The task decides the modality.** A new case takes the modality of its task. On an existing
   case, the task must be of the case's modality.
3. **Five tasks, as analysis types.** Code stays in English; the Portuguese names are for the
   screens and the reports.

   | Type | Portuguese | Modality | Report | Status |
   |---|---|---|---|---|
   | `FACIAL_REVIEW` | Análise facial (revisão) | FACIAL | lead report | as now |
   | `FACIAL_COMPARISON` | Comparação facial | FACIAL | laudo | as now |
   | `FINGERPRINT_COMPARISON` | Confronto papiloscópico | FINGERPRINT | laudo de confronto papiloscópico | phase 1 |
   | `FINGERPRINT_LAB_EXAMINATION` | Exame laboratorial | FINGERPRINT | laudo de exame laboratorial | coming soon: reserved, not offered |
   | `CRIME_SCENE_EXAMINATION` | Exame de local | FINGERPRINT | laudo de exame de local | coming soon: reserved, not offered |

   *Decided 2026-10-28:* the laboratory and scene examinations are out of scope for now. Each has a
   "coming soon" page, and routing lists them disabled. Their types are reserved so the database and
   the reports leave room for them.
4. **Tasks can follow one another on a case.** Each task is a request's own: a later request for
   a comparison joins the case and adds its analysis, answering that request. A laboratory
   examination that develops latents is followed by a comparison asked by the same request or
   another.
5. **A comparison can open without pairs; the coordinator chooses them.** Routing opens it from
   the request, facial or fingerprint. *Decided 2026-10-28:* the coordinator then adds its pairs
   from what the request sent, questioned traces and reference traces, and from the matcher's
   candidates on the case page (enrolled persons). Pairs can be added while the comparison is at
   its examination stage, before the first examiner submits.
6. **Fingerprint conclusions are positive, negative or inconclusive** (*decided 2026-10-28*):
   the decision itself, with no graded scale like the facial laudo's. In a laudo, a positive
   settled by the comparison's rounds is an identification.
7. **In the sim, requests send only the questioned evidence.** References come up on the case page
   through matching, and the coordinator adds the pairs from there.

## Changes

**Data (migration 013).**
- `case_analyses.analysis_type` and `analysis_reports.report_type` accept the new types. That
  includes `CRIME_SCENE_EXAMINATION`, which the API refuses for now.
- No new table: the analyses a request opens are its tasks, linked by `case_request_id`.

Migration 013 was reserved for the fingerprint processing plan's codification columns; that
plan moves to 014.

**`analyses`.**
- New `Type` values, with their `Modality` and `ReportType`.
- `Offered()` says which types routing may open: the two facial types and the fingerprint
  comparison.
- `Comparison()` names the types whose pairs a coordinator lists (both comparisons).
  `GradedConclusion()` names the facial comparison only.
- `Open` accepts every offered type. Pairs are required for none: a comparison may start empty.
- `AddPairs` adds pairs to a comparison at its examination stage.
- `OpenAutomatic` goes; `cases.Create` and `cases.Ingest` stop calling it.

**Examination and reports.**
- A fingerprint comparison uses the same worksheet, decision rounds and blind verification as
  a facial one. Its decisions carry no conclusion level.
- Its laudo is "Laudo de Confronto Papiloscópico": the facial laudo's structure, with the
  ACE-V method for friction ridges and the decision table in place of the graded scale.

**`requests`.**
- `CaseInput` gains `Task` (an analysis type) and loses `Modality`, which the task now
  determines.
- `Register` and `Create` open the task's analysis in the case, answering the request.
- An existing case with a task of another modality is refused.

**API and screens.**
- **Register a request:** each case row offers the task, by its Portuguese name in pt-BR,
  instead of the modality.
- **Follow-up request on a case:** same choice.
- **Requests card on a case page:** shows the task each request asked.
- **Requests list:** shows each request's tasks.
- **"New case" without a request:** stays. Its analyses are opened afterwards by a coordinator
  (`OpenAnalysisHandler`, for any offered type).

**trackid-sim** (its ROADMAP records "every facial case gets a FACIAL_REVIEW" as decided in
trackid, and `plant` works on those reviews).
- The request each loaded case answers names its task: `FACIAL_REVIEW` for every facial case,
  `FINGERPRINT_COMPARISON` for every fingerprint case.
- The class exercise is unchanged: every facial case still has its review, now because its
  request asked for one.
- The ROADMAP entry is updated to say so.

## Phases

1. **Routing with tasks, and the fingerprint comparison** (built 2026-10-28).
   - The types, migration 013, routing in `requests`, and the screens.
   - `OpenAutomatic` removed.
   - Comparisons opening without pairs, and pairs added by the coordinator.
   - The fingerprint comparison: worksheet, rounds and laudo.
   - "Coming soon" pages for the laboratory and scene examinations.
   - The sim's requests naming their tasks.
2. **Fingerprint comparison, further.** Suitability of fingerprint traces in the worksheet, and
   the image lab's processed images as the compared images (`docs/fingerprint-image-processing-plan.md`).
3. **Laboratory examination (exame laboratorial).**
   - The objects sent in.
   - The development methods applied to each (powders, ninhydrin, cyanoacrylate and others).
   - The latents developed, photographed and recorded as the case's evidence and lifts.
   - The laudo, and the comparison that follows.
4. **Scene examination (exame de local)**, later.

## Open

- **What a laboratory examination records per object and method,** and in what words for the
  laudo.

## Status (2026-10-28)

Phase 1 is built:

- **Backend:**
  - the task types and migration 013;
  - routing in `requests.Register` and `requests.Create`, with `Task`;
  - `analyses.OpenIn` and `AddPairs`, and the coordinator's `POST
    /api/admin/cases/:caseId/analyses/:analysisId/pairs`;
  - `OpenAutomatic` removed.
- **Fingerprint comparison:**
  - the same worksheet and rounds as faces, with no conclusion level;
  - a positive settled in its rounds counts as a laudo identification (`cluster.ReviewedLink`);
  - its laudo is "Laudo de Confronto Papiloscópico".
- **Screens:**
  - a task per case when registering a request, and on a follow-up request (laboratory and
    scene listed, disabled);
  - the tasks each request asked, on the case page and in the requests list;
  - the comparison modal adds pairs to the open comparison, or opens a new one;
  - the case's matcher candidates show on fingerprint cases too, each with "Add to comparison";
  - the "coming soon" pages, under "Fingerprint examinations" in the sidebar.
- **trackid-sim:** `load` asks a facial review of every facial case and a fingerprint
  comparison of every fingerprint case.

Checked:

- **Tests:**
  - routing (tasks named, offered, of the case's modality);
  - the request's empty comparison and `AddPairs`;
  - a fingerprint comparison's drafts and rounds;
  - the fingerprint laudo's text;
  - `ReviewedLink` for fingerprint comparisons;
  - migration 013 down and up.
- **A scratch load of trackid-sim's world:**
  - 189 facial reviews and 64 fingerprint comparisons, each answering its request;
  - on SIM-2021-00025, the lift's candidate (SourceAFIS 134.6) added to the comparison and shown
    in its worksheet beside the enrolled ten-print.
