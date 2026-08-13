# Paper 3 (Future): Forensic Evidentiary Validation

> **Status**: Not started. This paper is staged for after Paper 1 (Tactical Case Linking) and Paper 2 (Strategic Intelligence) are submitted. It absorbs the "slow," court-admissible evidentiary workflow that was originally scoped into Paper 1 before the pivot to a pure tactical/DSS framing — see `docs/RESEARCH_STRATEGY.md` for the pipeline rationale.

## Title (Working)
**From Tactical Lead to Court-Admissible Evidence: A FISWG-Compliant ACE-VR Verification Layer for Facial Comparison Decision Support**

## Relationship to Paper 1

Paper 1 establishes the **Tactical Queue**: a fast, low-friction mechanism by which analysts confirm or dismiss automatically-proposed cross-case links (one-second accept/reject clicks, no formal documentation, no legal weight). That queue is explicitly scoped as *operational intelligence only* — its outputs are leads, not evidence.

This paper picks up where a Tactical Queue confirmation is *not* sufficient: when a proposed identification needs to become part of a court-admissible case file. It introduces a second, deliberately slow and heavyweight verification tier — cognitively and procedurally the opposite of Paper 1's fast triage — grounded in Kahneman's Dual-Process Theory (System 1 vs. System 2 cognition) and forensic facial-comparison standards (FISWG, ACE-VR).

**Framing for reviewers**: "Paper 1 established a fast, low-friction Decision Support architecture for tactical case linking, explicitly scoped to operational intelligence rather than evidentiary use. This paper addresses the complementary problem: when a tactical lead must be escalated to a court-admissible identification, what verification architecture prevents automation bias while remaining tractable for practitioners?"

## Abstract Skeleton

Automated facial comparison tools risk introducing automation bias when their suggestions are treated as evidence without rigorous, structured human verification. This paper presents a Decision Support System (DSS) verification tier — layered on top of an existing tactical case-linking architecture — that enforces the full ACE-VR (Analysis, Comparison, Evaluation, Verification) methodology with a mandatory FISWG morphological checklist and blind second-expert review, producing immutable, court-admissible identification reports. Grounded in Kahneman's Dual-Process Theory, the design forces examiners out of fast, System-1 pattern-matching acceptance and into slow, System-2 structured analysis. Evaluation with N law-enforcement examiners demonstrates that FISWG-enforced review increases decision time by X% but reduces false-positive identification errors by Y%, and that mandatory blind dual-expert verification achieves Z% agreement. The contribution is a UI- and workflow-driven behavioral intervention against automation bias — not an improvement to the underlying facial comparison algorithm.

## Key Contributions

1. **Full ACE-VR Workflow Enforcement** (Forensic Governance Contribution):
   - **Analysis Phase**: system-logged assessment of image quality, resolution, inter-pupillary distance, aspect ratio, lighting, and camera artifacts before any comparison begins.
   - **Comparison Phase** (FISWG Morphological Checklist — Mandatory): structured, checkbox-enforced comparison across anatomical features (ear shape/lobe attachment, hairline pattern, nose bridge/tip/asymmetry, scars/moles/tattoos, chin shape, cheekbone structure).
   - **Evaluation Phase**: a formal conclusion — Identification / Inconclusive / Exclusion — with documented rationale.
   - **Verification Phase**: mandatory blind second-expert repeat of the full Analysis → Comparison → Evaluation cycle; disagreements flagged for joint resolution; both examiners digitally sign.
   - **Metric**: ACE-VR compliance rate (target 100% — no shortcuts), dual-expert agreement rate.

2. **Automation Bias Mitigation via Structured Friction** (Behavioral Contribution):
   - Problem: examiners tend to rubber-stamp high-similarity-score suggestions (System 1 acceptance) rather than independently verifying them (System 2 analysis).
   - Solution: the FISWG checklist is not optional guidance but a UI-enforced gate — the system will not accept a conclusion until every anatomical feature is documented.
   - **Metric**: false-positive identification error rate, decision time, examiner confidence, compared against an unstructured baseline review.
   - **Novelty**: this is a UI-driven behavioral intervention, not an algorithmic fix — the paper's central claim is that DSS *workflow design* is what prevents automation bias, not model accuracy.

3. **Escalation Path from Tactical Lead to Evidentiary Record** (Architectural Contribution):
   - Problem: unvalidated automated suggestions reaching a judge, or operational hunches contaminating court evidence, are both failure modes reviewers flag as high-risk for AI-assisted investigation tools.
   - Solution: clean separation — a Tactical Queue confirmation (Paper 1) can *nominate* a case for evidentiary review, but only a completed, dual-signed ACE-VR report is admissible. The two tiers never share output format or legal weight.
   - **Metric**: audit trail completeness; rate at which Tactical Queue leads are subsequently escalated and how many survive full ACE-VR verification (a proxy for how much operational speed differs from evidentiary rigor).

## Methodology (Outline)

### The Verification Tier

**Workflow** (Complete Analysis-Comparison-Evaluation-Verification Cycle) — see full detail preserved from the original design in the sections below.

1. **Analysis Phase**: image quality, IPD, aspect ratio, lighting, camera artifacts logged by the system.
2. **Comparison Phase** (FISWG Morphological Checklist, mandatory completion): ear shape/lobe attachment, hairline pattern, nose (bridge, tip, asymmetry), scars/moles/tattoos, chin shape/dimple, cheekbone structure.
3. **Evaluation Phase**: formal conclusion (Identification / Inconclusive / Exclusion) with documented rationale.
4. **Verification Phase** (mandatory blind peer review): a second, independent expert — blind to the first examiner's checklist — repeats the full cycle; disagreements are flagged for joint resolution; both examiners digitally sign off.
5. **Report Generation**: immutable PDF/JSON Forensic Identification Report containing complete FISWG checklists from both examiners, image provenance (source, timestamp, extraction method), full audit trail, dual-expert verification logs, and case linkage metadata (including the originating Tactical Queue confirmation, if any).

**Cognitive Mode**: System 2 — analytical, slow, error-intolerant, fully documented. Deliberately the opposite of Paper 1's Tactical Queue (System 1, fast, low-friction).

### Evaluation Design

Unlike Paper 1, this paper's core claims (automation bias mitigation, false-positive reduction under structured review) are behavioral and require a **human-subjects examiner study** — this cannot be simulated on a public dataset the way Paper 1's silo-breaking metrics were.

- **Participants**: N law-enforcement or trained examiners (suggest N ≥ 10); requires appropriate ethics/IRB approval given human-subjects involvement.
- **Conditions**: (1) traditional/unstructured review (baseline); (2) FISWG-enforced ACE-VR review (TrackID verification tier).
- **Metrics**: ACE-VR compliance %, dual-expert agreement rate, morphological checklist completion %, false-positive identification error rate, decision time, examiner confidence (5-point scale).
- **Statistical rigor**: confidence intervals on all rates; paired t-tests on decision time (structured vs. unstructured); inter-rater reliability (Fleiss'/Cohen's κ) on examiner agreement.

### Results Structure (Placeholder)

**Table 1**: ACE-VR / FISWG Compliance and Rigor
| Metric | Result | Target |
|---|---|---|
| ACE-VR Compliance | — | 100% |
| Dual-Expert Agreement Rate | — | >90% |
| Morphological Checklist Completion | — | 100% |
| False-Positive Identification Error Rate | — | ~0% |
| Decision Time (structured vs. baseline) | — | report Δ% |
| Examiner Confidence (1–5) | — | >4.5 |

## Literature Review (Outline)

- **Automation Bias & Dual-Process Theory**: Kahneman, 2011; Parasuraman & Riley, 1997; Wickens & Hollands, 2000.
- **FISWG & Forensic Protocol Compliance**: FISWG facial comparison guidelines; Grother et al., 2019 (NIST facial analysis documentation); Dror & Mnookin, 2010 (cognitive bias in forensic science).
- **Human-Centered AI / Structured Decision Support**: Amershi et al., 2019; Shneiderman, 2022.
- **Legal Admissibility of Algorithmic Evidence**: relevant case law and forensic-science-in-the-courtroom literature (to be scoped).

## Discussion Points

1. **Why Structured Friction Works**: forcing examiners through a checklist overrides the automation-acceptance heuristic — hypothesized behavioral mechanism, tested via decision-time and error-rate shifts.
2. **Deskilling Concern**: does outsourcing candidate discovery to a tactical layer (Paper 1) deskill examiners who now only verify curated candidates? Argue no — this tier still requires full independent analysis, not confirmation of the tactical suggestion.
3. **Where Legal Weight Actually Lives**: only this tier's dual-signed reports carry evidentiary weight; Tactical Queue confirmations from Paper 1 explicitly do not, and the paper should make this boundary legible to both technical and legal reviewers.

## Limitations & Future Work

- Requires human-subjects study design and ethics approval, unlike Papers 1 and 2.
- Dataset/jurisdiction-specific; cross-jurisdictional validation of thresholds and protocols needed.
- Real-time performance and integration overhead with the Tactical Queue escalation path not yet evaluated.

## Target Journals & Keywords

**Primary Targets**:
1. **Forensic Science International: Digital Investigation** — best fit for FISWG/ACE-VR/legal admissibility framing.
2. **IEEE Transactions on Human-Machine Systems** — automation bias / HCI angle.
3. **ACM Transactions on Computer-Human Interaction (ToCHI)** — if reframed primarily as a UI-driven behavioral intervention.

**Keywords**: automation bias, FISWG, ACE-VR, forensic facial comparison, dual-process theory, System 1/System 2, examiner verification, legal admissibility, cognitive bias in forensics.

**Avoid Framing As**: face recognition accuracy benchmarking, biometric matching optimization — this is a workflow/behavioral contribution, not a model contribution.
