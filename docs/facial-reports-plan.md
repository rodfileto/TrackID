# Facial reports: requests, leads, laudos

Decided 2026-10-26. Facial only, and only the passive flow: an authority sends material (images or
video) and asks questions; the institute answers with a lead report (`FACIAL_REVIEW_REPORT`) and,
when asked, a laudo (`FACIAL_COMPARISON_REPORT`). Crime scene, laboratory and fingerprint reports
are left for later. Documents are in Portuguese; code, identifiers and the UI stay in English.

## Decisions

1. **One analysis, one report.** A case has several reports because it has several analyses
   (`case_analyses` → `analysis_reports`, as now). The report is the formal output of its
   analysis; assignment, numbering and access stay with the analysis.
2. **Requests are their own records** (`case_requests`), not part of the occurrence. The
   occurrence is the event (POLE Event); a request is an administrative act about it. One
   occurrence can lead to several requests (police station, prosecutor, court), and a civil case
   has a request but no occurrence. A request holds the requesting authority, its document
   (e.g. ofício no.), the date received, the due date, the occurrence or inquiry number it cites,
   and its questions (quesitos, an ordered list). Registering a request finds or creates the
   occurrence it cites and links it, on one screen. Every analysis names the request it answers;
   the laudo answers that request's questions. Requests are registered by a coordinator.
3. **Received material** is the request's files (`case_files`, with a SHA-256 hash and the date
   received): the minimum chain of custody for material that was sent in.
4. **Reference traces.** Images sent in for comparison (e.g. a suspect's photo) become traces of
   the case with `trace_role = REFERENCE`; the material to identify is `QUESTIONED`. A reference
   is never listed as an unidentified face, and the laudo separates questioned and reference
   material. A comparison pair is questioned × reference (or × enrolled person), so it stays
   inside the case and uses `case_analysis_pairs`.
5. **Suitability (FISWG analysis phase).** The examiner records each questioned trace's value:
   `SUITABLE` (for comparison), `LEAD_ONLY` or `UNSUITABLE`, with a note. This is what lets a
   positive lead coexist with "image has no quality for comparison": the lead report can say so,
   and a comparison is not opened on a trace that cannot conclude (the coordinator is warned).
6. **Two conclusion scales.**
   - *Lead* (review): `POSITIVE` / `NEGATIVE` / `INCONCLUSIVE`, as now.
   - *Laudo* (comparison): a new `conclusion` on the human decision, on a graded scale
     (working list, to be fixed with the institute: `IDENTIFICATION`, `STRONG_SUPPORT`,
     `MODERATE_SUPPORT`, `LIMITED_SUPPORT`, `INCONCLUSIVE`, `LIMITED_EXCLUSION`, `EXCLUSION`).
     Every level maps onto `decision` (support levels and identification → POSITIVE, inconclusive
     → INCONCLUSIVE, exclusions → NEGATIVE), so `cluster.DecidingRound` and the rounds logic keep
     working unchanged. Required on decisions of a FACIAL_COMPARISON, forbidden elsewhere.
7. **Two cluster layers stay; each human link carries its basis.** *Re-evaluated 2026-10-26:*
   when a pair's only human round is a review, the person profile keeps it as identified, so a
   lead must stay in the layer the person views read. A separate EXAMINED layer is therefore not
   needed: the REVIEWED layer already holds the latest settled human round of each pair
   (`cluster.DecidingRound`, across analyses), and what that round was says how strong the link is.
   - Each REVIEWED link gets a **basis**, derived from the deciding round's analysis type:
     `LEAD` (a FACIAL_REVIEW) or `LAUDO` with its conclusion level (a FACIAL_COMPARISON).
   - Precedence follows the existing rounds rule: a comparison that concludes (any POSITIVE or
     NEGATIVE level) replaces the lead; one that is pending or ends INCONCLUSIVE does not undo it,
     and the profile shows the laudo's inconclusive result beside the lead; an exclusion rejects
     the pair and it leaves the layer.
   - **Identified** on the person profile, case page and search badges: every REVIEWED link,
     labelled *identified (lead)* or *identified (laudo, <level>)*. A laudo below the minimum level
     (default `STRONG_SUPPORT`, set as data like `match_thresholds`) shows its level and is
     labelled *supported*, not identified.
   - A REVIEWED cluster can join traces through links of both bases; the profile shows each link's
     basis, never a cluster-wide one. "Identified by laudo only" is a filter on the basis, not a
     layer.
   - `reviewed-clusters-plan.md` decisions 6–8 stand unchanged.
8. **Process section of the laudo** records the whole chain: examiner, verifier, and, when they
   disagreed, that they did and how the third examiner settled it (from the analysis's decisions
   and their `notes`). Nothing new to store.
9. **Editing and rendering with WordCraft, embedded.** The examiner edits the report in
   WordCraft's web (WASM) build, in a frame on the analysis page, because its on-screen
   pagination is the printed one. We build the embedding ourselves.
   - The server generates the draft `.docx` from a Portuguese template, with the data sections
     filled in (request, questions, material, method, pairs and conclusions, process).
   - The examiner writes the text sections (objective, description of the material, analysis per
     pair, answers to the questions); saving sends the `.docx` back as a new draft version.
   - **Issuing is server-side**: TrackID checks that the data sections are identical to the
     data snapshot (refusing to issue otherwise), converts the `.docx` to PDF with
     `wordcraft-cli convert` (same engine, same pagination), stores docx, PDF and snapshot as
     `forensic_report` case files with their hashes, numbers the report and closes the analysis.
     The PDF never comes from the browser.
   - The template's fonts are bundled in both the web build and the server.
10. **Issued reports never change.** A correction is a new report, a *laudo complementar*,
    pointing at the one it complements; the original stays.
11. **Signature**: issuer and verifier recorded plus the PDF hash. ICP-Brasil digital signature
    is a later option, not designed against.
12. **The lead report is numbered and issued** like the laudo (as now), and states that it is an
    investigative lead, not an identification.
13. **Delivery is recorded**: when, how and to whom an issued report was sent.
14. **Video (later phase)** through `trackid-vision/videoproc`: the video is received material;
    each track becomes an evidence; frames the examiner picks become traces recording the video,
    timestamp and frame number. Needs the full frame stored (videoproc keeps only a data-URI
    crop) and ffmpeg on the worker host.

15. **Laudo structure** and **executive summary**: see the next section. The structure is ours,
    to be adjusted once the institute's own model is known.

## Laudo structure (`FACIAL_COMPARISON_REPORT`)

Headings in Portuguese. *Data* sections are generated and locked (checked at issue, decision 9);
*text* sections are the examiner's.

| # | Seção | Kind | Content |
|---|---|---|---|
| — | Cabeçalho / rodapé | data | institute, "Laudo de Comparação Facial nº <ano>.<seq>", case number, page *n* de *N*, PDF hash on the last page |
| 0 | **Sumário executivo** | data | one page, see below |
| 1 | Preâmbulo | data + text | date, issuing examiner, request that ordered it (authority, document, date received); fixed legal sentence editable only in the template |
| 2 | Histórico | data | the request, the occurrence it cites, previous reports of the case (the lead report and any complements, by number) |
| 3 | Objetivo | text | default from the template: "Realizar exame de comparação facial entre…", pre-filled with the pairs |
| 4 | Material | data | 4.1 questioned material (each trace: source file, hash, evidence, capture data, suitability); 4.2 reference material (sent-in references and enrolled records used: document, enrollment date, file hash) |
| 5 | Método | data + text | ACE-V and FISWG morphological analysis, the conclusion scale with the meaning of each level, tools (TrackID version; matcher and model version only as how candidates were found, never as the conclusion) |
| 6 | Exames | text (per pair, under data headers) | for each pair: images side by side (data), then the examiner's analysis: quality and limits, features compared, similarities, differences and their explanation |
| 7 | Processo de verificação | data | examiner, blind verifier, their conclusions, disagreement and its resolution by the third examiner with their notes (decision 8) |
| 8 | Conclusão | data + text | each pair's final level (data) and a short discussion (text) |
| 9 | Resposta aos quesitos | text (per question, under data headers) | each quesito as received, then its answer |
| 10 | Encerramento | data | number of pages and annexes, examiner and verifier names and staff numbers, signature lines, hash |
| — | Anexos | data | full-resolution images, if not in section 6 |

The lead report (`FACIAL_REVIEW_REPORT`) uses a short form: header, executive summary,
preâmbulo, material (questioned only), método (system, model and threshold version, candidates
examined), resultado per face (lead to person *P* / no lead / unsuitable), the fixed notice
"Este relatório constitui informação de inteligência / indicação investigativa e não constitui
identificação", encerramento.

## Executive summary (`sumário executivo`)

Built entirely from the database at issue time and frozen in the snapshot; the examiner cannot
edit it, only read it in the draft. One page, placed after the header, so a reader gets the
result without the technical sections. The same generator renders it in the UI (analysis page
and case page) before the report is issued, marked *preliminar*.

| Field | Source |
|---|---|
| Report | type, number, issue date (*preliminar* until issued) |
| Case | case number, case type, occurrence number and classification |
| Request | authority, document number, date received, due date, number of quesitos |
| Material | questioned traces: count by suitability (suitable / lead only / unsuitable), source files (images, videos); reference material: count by kind (sent in / enrolled) |
| Results | one row per pair: questioned trace → reference (person name and document when enrolled), conclusion level, *identified / supported / inconclusive / excluded* |
| Verification | examiner and verifier agreed on every pair, or *n* pairs settled by a third examiner |
| Previous reports | the lead report number and its result for the same traces, and whether the laudo confirms, changes or leaves it standing (the rounds rule, decision 7) |
| Examiners | examiner, verifier, third examiner when there was one |

The lead report's summary has the same layout, with the lead result per face instead of
conclusion levels, and the investigative-lead notice in it.

## Phases

0. **WordCraft spike** (before phase 4, can run in parallel with 1–3). In our own page: build
   the web build, load a `.docx` from a URL into a frame, get the saved document back to the
   host page (postMessage bridge in our fork if there is no API), find what "restrict editing"
   protects, and compare the browser's pagination with `wordcraft-cli convert` on the server.
   Outcome: a go/no-go note here; on no-go, fall back to Typst rendering with Markdown text
   sections.
1. **Requests and material.** Migration 009: `case_requests` (with ordered questions),
   `case_analyses.case_request_id`, received files linked to their request with hash and date,
   `case_traces.trace_role`. `cases` package and API; one registration screen that finds or
   creates the cited occurrence. Sim: `world.json` gains requests and reference traces.
2. **Suitability.** Per-trace value on the trace with who and when; shown in the review
   workspace; warning when opening a comparison on a non-SUITABLE trace.
3. **Conclusion scale and link basis.** `conclusion` on `biometric_decisions` and on the
   drafts, with its mapping to `decision` in Go and a CHECK; the minimum level as data. The
   deciding round reports its basis (LEAD / LAUDO + level); `cluster_members` or the link views
   carry it, Neo4j `IDENTIFIED_AS` gets a `basis` property; case page, person profile and badges
   show the labels. Sim `eval` scores identifications per basis.
4. **Report content and issuing.** Template (Portuguese) and docx generation in Go; draft
   versions; data snapshot and the identical-sections check; server-side PDF; hashes; complement
   reports; delivery record. Embedded editor in the analysis page.
5. **Video.** videoproc ingestion as above.

## Docs

MODEL.md sections 2.2, 3, 3.1 and 4.1 (link basis); the book chapters on analyses and clusters.

## Open

- The institute's conclusion scale and the minimum level for EXAMINED.
- The laudo's required structure (preamble, history, objective, exams, discussion, conclusion,
  answers to questions); a blank or anonymized model, never a real report.
