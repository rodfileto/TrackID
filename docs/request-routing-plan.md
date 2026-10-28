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
   | `FINGERPRINT_LAB_EXAMINATION` | Exame laboratorial | FINGERPRINT | laudo de exame laboratorial | phase 3 |
   | `FINGERPRINT_COMPARISON` | Confronto papiloscópico | FINGERPRINT | laudo de confronto papiloscópico | phase 2 |
   | `CRIME_SCENE_EXAMINATION` | Exame de local | FINGERPRINT | laudo de exame de local | later: reserved, not offered |

   The scene examination is out of scope for now. The type is reserved so the database and the
   reports leave room for it.
4. **Tasks can follow one another on a case.** Each task is a request's own: a later request for
   a comparison joins the case and adds its analysis, answering that request. A laboratory
   examination that develops latents is followed by a comparison asked by the same request or
   another.
5. **A facial comparison can open without pairs.** Routing opens it from the request: the
   questioned traces are the case's, and the reference images come with the request (reference
   traces). Its pairs are then chosen within the analysis, by the coordinator or the examiner,
   instead of being required at opening.

## Changes

**Data (migration 013).**
- `case_analyses.analysis_type` and `analysis_reports.report_type` accept the new types. That
  includes `CRIME_SCENE_EXAMINATION`, which the API refuses for now.
- No new table: the analyses a request opens are its tasks, linked by `case_request_id`.

Migration 013 was reserved for the fingerprint processing plan's codification columns; that
plan moves to 014.

**`analyses`.**
- New `Type` values, with their `Modality` and `ReportType`.
- `Offered(t)` says which types routing may open: everything but `CRIME_SCENE_EXAMINATION`.
- `Open` accepts every offered type. Pairs are required for none: a comparison may start empty.
- `OpenAutomatic` goes; `cases.Create` and `cases.Ingest` stop calling it.

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

1. **Routing with tasks.**
   - The types, migration 013, routing in `requests`, and the screens.
   - `OpenAutomatic` removed.
   - Facial comparison opening without pairs.
   - The sim's requests naming their tasks.
   - Fingerprint analyses can be opened and assigned, but have no worksheet yet.
2. **Fingerprint comparison (confronto papiloscópico).**
   - The analysis worksheet: the matcher's candidates (SourceAFIS), the pairs, the trace's
     suitability.
   - The decision rounds tied to the analysis, as for faces (examination, verification,
     inconsistency).
   - Its conclusion scale and its laudo template.
3. **Laboratory examination (exame laboratorial).**
   - The objects sent in.
   - The development methods applied to each (powders, ninhydrin, cyanoacrylate and others).
   - The latents developed, photographed and recorded as the case's evidence and lifts.
   - The laudo, and the comparison that follows.
4. **Scene examination (exame de local)**, later.

## Open

- **Conclusion scale for fingerprint comparisons:** identification, exclusion, inconclusive, or
  a graded scale like the facial laudo's. The institute's practice decides.
- **What a laboratory examination records per object and method,** and in what words for the
  laudo.
- **Who may choose a comparison's pairs:** the coordinator only, or also the examiner within
  their own analysis.
