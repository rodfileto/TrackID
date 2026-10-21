---
title: "TrackID — Book Plan"
subtitle: "A Quarto book on forensic intelligence, ACE-V and collective intelligence, and the system that puts them together"
author: Rodrigo Fileto Cuerci Maciel
lang: pt-BR
---

## Premise

- **What the book is**: a course companion for the review exercise that a forensic examiner or
  analyst can also read on their own. Each theoretical chapter is followed by how TrackID
  implements it, and the book ends with the exercise itself: how a class works one synthetic world
  in TrackID and how its decisions are scored against the ground truth.
- **Audience**: students (forensic science, police and identification courses) and practitioners
  (papiloscopistas, peritos, intelligence analysts). No programming assumed; code and tables are
  named so a technical reader can follow them, never needed to understand the argument.
- **Language**: pt-BR. English terms are kept where the literature uses them (ACE-V, FISWG, blind
  verification), with the Portuguese term at first use and in a glossary.
- **Formats**: HTML (browsed in class) and PDF (cited), one source, one BibTeX bibliography, ABNT
  citation style.
- **Where**: `quarto/book/` in trackid, a sibling project as `quarto/README.md` asks, alongside
  `technical-report/` and `research-plan.md`.
- **Book first** (decided 2026-10-16): the book is written before the papers and the technical
  report, which will be handled later and may draw on it.
- **Public sources only** (decided 2026-10-16): the book describes no institutional procedure (no
  PF/INI practice); every claim rests on published, citable sources and on TrackID itself.

### The argument in one paragraph

Identifying people from biometric traces has been organized as a case-by-case service: one
request, one comparison, one laudo. Forensic intelligence asks for the opposite view -- traces
linked across cases, series and unknown subjects that only appear in the aggregate. ACE-V is the
method that keeps each comparison defensible; collective intelligence is what turns many examiners'
decisions, each fallible, into a reliable shared result. TrackID is built on the three at once:
every link between traces is a decision with a recorded author and method (ACE-V), decisions are
distributed and checked by independent examiners (collective intelligence), and the links they
confirm build clusters and series that no single case shows (forensic intelligence).

## Structure

Three parts, one per question: *why* (theory), *how* (the system), *with what* (the exercise).

### Pré-textuais

- **Prefácio** -- why the book exists; how to read it (students: in order; practitioners: Part I,
  then chapter 8; developers: Part II with `MODEL.md`).
- **Convenções** -- terms, notation for decisions and pairs, how code and tables are referenced.

### Parte I -- Fundamentos

| # | Chapter | Covers | Key sources (to verify) |
|---|---|---|---|
| 1 | **Identificação de pessoas e seus silos** | Identification services in general terms: enrollment (civil/criminal), questioned traces, case-by-case requests; how fingerprint and face work, AFIS/ABIS and case files end up as separate silos. Sets up the problem, with no institution's own procedures. | published literature on forensic identification and forensic intelligence (Ribaux; NAS 2009) |
| 2 | **Inteligência forense** | Forensic intelligence as a discipline: traces as information beyond the court case; linking cases, series, unknown subjects; the intelligence cycle; place and time (occurrences, maps). The difference between evidence for a trial and information for intelligence, and why the same decision can serve both. | Ribaux & Margot; Ribaux, Walsh & Margot (2006); Rossy et al.; Ribaux (*Police scientifique*, 2014); Morelato et al. |
| 3 | **ACE-V e a comparação forense** | Analysis, Comparison, Evaluation, Verification: origin (fingerprints), extension to faces; one-to-many review vs one-to-one comparison (FISWG); conclusion scales; verification -- independent, blind; conflict resolution. Cognitive bias and contextual information: why blindness and sequencing matter. The critiques (NAS 2009, PCAST 2016) and the responses (black-box studies, OSAC standards). | Ashbaugh (1999); Huber; FISWG (one-to-many v1.0 2026, one-to-one, facial comparison overview); NAS (2009); PCAST (2016); Dror & Charlton (2006); Dror et al. (2015, LSU); Ulery et al. (2011, 2012) |
| 4 | **Inteligência coletiva** | When many fallible judgements beat one: independence, diversity, aggregation; wisdom of crowds vs groupthink; collective intelligence in organizations. Evidence from forensic comparison itself: examiners, super-recognizers and algorithms combined (fusion) outperforming any of them alone. What this asks of a workflow: independent decisions, recorded disagreement, a rule to settle it. | Surowiecki (2004); Hong & Page (2004); Woolley et al. (2010); Malone & Bernstein (2015); Phillips et al. (2018, PNAS); White et al. (2015) |
| 5 | **Humanos e máquinas na identificação** | The matcher's place: scores, thresholds as data, the review band; automation bias; what a score is and is not (no likelihood ratio without validation); who is accountable for a SYSTEM decision. Bridges Part I to Part II. | NIST FRVT/FRTE reports; ISO/IEC 19795; Phillips et al. (2018); literature on automation bias |

### Parte II -- O sistema

Each chapter opens with the concept from Part I it implements, and closes with the screen where a
user meets it. Chapters reference `MODEL.md` sections rather than repeating them.

| # | Chapter | Covers | In TrackID |
|---|---|---|---|
| 6 | **O modelo de dados: vestígios, características e decisões** | Two hierarchies (enrollment KNOWN, evidence QUESTIONED) meeting in the biometric feature; a decision is always between two features; the decision log as source of truth; clusters as a derived, citable view; occurrences and places. | `MODEL.md` §1-5; `biometric_decisions`, `clusters`, `occurrences` |
| 7 | **Do escore à decisão** | The matcher's SYSTEM decisions; threshold versions with review band; what is confirmed automatically and what goes to people. | `match_thresholds`, `biometricmatch`, `cmd/match-threshold` |
| 8 | **ACE-V no TrackID: análises, etapas e laudos** | Analyses as rounds (FACIAL_REVIEW automatic, FACIAL_COMPARISON on demand); stages EXAMINATION -> VERIFICATION (blind) -> INCONSISTENCE -> REPORT; drafts until submitted; the report and its number; why a pair keeps its latest settled outcome across rounds. Maps each stage to ACE-V and FISWG. | `analyses`, `examination`, `cluster.DecidingRound`; `db/migrations/006_analyses.sql`; the analysis workspace |
| 9 | **Distribuição e papéis** | User profiles (admin vs examiner, modalities); distribution of each step, never two steps of one analysis to one person; least-loaded rule; access by assignment; blind verification in the interface. The design choices and their reasons (independence, accountability, workload). | `auth`, `distribution`; Distribution and My analyses screens |
| 10 | **Da decisão à inteligência** | How confirmed links become clusters, identifications and series; the occurrence map; what changes when a decision is revised (merges, splits, stable cluster ids). Closes the loop to chapter 2. | `cluster.Run`, `cluster.Identify`; Clusters and Occurrence map screens |
| 11 | **Auditoria, privacidade e limites** | Append-only logs and who decided what; LGPD and biometric data; what the system does not do (no LR, cross-case pairs not distributed yet, fingerprint analyses not modelled yet). | research plan P7; `MODEL.md` |

### Parte III -- O exercício

| # | Chapter | Covers | In trackid-sim |
|---|---|---|---|
| 12 | **Um mundo sintético com verdade conhecida** | Why synthetic: no real people, a ground truth to score against; how the world is made (population, names, enrollments, offending, occurrences in Rio, faces and prints we generate); what synthetic data can and cannot show. | `docs/PLAN.md`, `docs/OCCURRENCE.md`, `docs/LICENSES.md`; `generate`, `load` |
| 13 | **Roteiro da aula** | Step by step for the instructor and the students: start the stack, create users, distribute, examine, verify blind, resolve inconsistencies, issue reports; screenshots of each screen. | `users` command; trackid screens |
| 14 | **Avaliando as decisões** | Scoring students' decisions against the truth (per student, genuine/impostor, SYSTEM band); the cluster changes they caused; collective vs individual accuracy -- the empirical side of chapter 4. | `eval`; block D items still to build (per-decision scoring, snapshot comparison) |
| 15 | **Discussão e próximos passos** | What the class shows, and what it cannot; open problems (fingerprint analyses, cross-case pairs, calibration). | research plan, roadmap |

### Pós-textuais

- **Glossário** (pt-BR / English / role in TrackID): análise, comparação, avaliação, verificação,
  verificação cega, inconsistência, vestígio, característica, par, cluster, sujeito desconhecido,
  laudo, relatório, revisão facial, comparação facial...
- **Referências** (one `references.bib`).
- **Apêndice A** -- Instalação do ambiente (trackid + trackid-sim).
- **Apêndice B** -- Referência rápida dos papéis e telas.

## Conventions

- **Terms**: the database role names stay in the system chapters (VERIFICATOR, REVIEWER,
  INCONSISTENCE) but are always glossed with the FISWG/ACE-V term (exame, verificação,
  inconsistência), which the screens already use. Chapter 3 explains the mismatch once (our
  VERIFICATOR is FISWG's practitioner, our REVIEWER is FISWG's verification).
- **Figures**: Mermaid for flows (stages, distribution), screenshots from a sim world (never real
  data), maps from the occurrence layer. Every figure has a source note.
- **Code**: named, not reproduced -- `distribution.Distribute`, `analyses.CanWork` -- with a link to
  the repository at a tagged version, so the book does not drift from the code.
- **Citations**: `references.bib`, ABNT style (`abnt.csl`). Every source in the tables above is a
  candidate and must be checked against the original before it is cited.
- **Callouts**: "Na prática" (how a practitioner meets the concept), "No TrackID" (where it lives),
  "Limite" (what synthetic data or the current system cannot show).

## Production

- **Project**: `quarto/book/_quarto.yml` with `project: type: book`, `lang: pt-BR`,
  `format: html` (theme, search, cross-refs) and `pdf` (LaTeX, ABNT-ish template),
  `bibliography: references.bib`, `csl: abnt.csl`.
- **Files**: `index.qmd` (prefácio), one `.qmd` per chapter in `parte-1/`, `parte-2/`, `parte-3/`,
  `glossario.qmd`, `references.qmd`, `apendices/`, `figures/`.
- **Screenshots**: captured from a fixed sim world (seed recorded) after the exercise works end to
  end, so they can be regenerated.
- **Versioning**: the book states the TrackID and trackid-sim versions it describes; Part II is
  revised when `MODEL.md` changes.
- **Keep local**: drafts and renders stay in the repo; nothing is published until decided.

## Milestones

| # | Milestone | Done when |
|---|---|---|
| M1 | **Skeleton** | `quarto render` builds HTML and PDF with all chapter files (titles + 3-5 line abstracts), glossary stub, `references.bib` with the candidate sources. |
| M2 | **Part I** | Chapters 1-5 drafted; every citation checked against its source. |
| M3 | **Part II** | Chapters 6-11 drafted against the current `main`; Mermaid figures for stages and distribution. |
| M4 | **Part III** | Chapters 12-14 after a full class run on a sim world; screenshots; chapter 14 needs the per-decision scoring (roadmap block D). Chapter 12 does not need the class and was drafted first (2026-10-20), its sources still to check. |
| M5 | **Review** | Read by a practitioner and a student; chapter 15 and the preface written last. |

Order: M1 -> M3 (the system is fresh) -> M2 -> M4 -> M5. Part II first because it is the part only
we can write and the one that changes with the code; Part I can lean on it for examples.

## Open questions

- **Licence of the book** (e.g. CC BY 4.0) and whether it is published with the trackid release.

Settled 2026-10-16: the book comes first (papers and the technical report later); no PF/INI
procedures, public sources only.
