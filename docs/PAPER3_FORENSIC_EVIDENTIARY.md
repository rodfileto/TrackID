# Paper 3 (Future): Evidentiary-Grade Verification

> **Status**: Not started. Staged for after Paper 1 (Person-Target Profiles) and Paper 2 (Situation/Network-Target Profiles) are submitted. See `docs/RESEARCH_STRATEGY.md` for the overall pipeline rationale.

## Title (Working)
**From Investigative Lead to Court-Admissible Evidence: A Structured Verification Layer for Person-Target Profiles**

## Scope & Positioning

Paper 1 builds a Person-Target Profile continuously and asynchronously from fragmented, multi-source evidence (biometric auto-match, analyst validation, field-officer document confirmation) — explicitly scoped as an *operational intelligence lead*, not evidence. This paper addresses the forensic/legal gap Paper 1 deliberately defers: what happens when a Person-Target Profile needs to become part of a court-admissible case file?

It introduces a separate, deliberately slow and heavyweight verification tier — cognitively and procedurally the opposite of Paper 1's fast, low-friction routing — grounded in Dual-Process Theory (System 1 vs. System 2 cognition) and forensic facial-comparison protocol standards (e.g. FISWG, ACE-VR).

**Framing for reviewers**: "Paper 1 establishes a fast, low-friction target-centric architecture for constructing identity-level Person-Target Profiles, explicitly scoped to operational intelligence rather than evidentiary use. This paper addresses the complementary problem: when such a profile must be escalated to a court-admissible identification, what verification architecture prevents automation bias while remaining tractable for practitioners?"

## Core Problem

Automated facial-comparison suggestions risk automation bias — examiners rubber-stamping a high-confidence system suggestion (System 1 acceptance) rather than independently verifying it (System 2 analysis). Legal defensibility requires structured comparison, full audit trails, chain-of-custody, and independent sign-off — none of which a fast tactical confirmation provides or should provide.

## Approach

A dedicated verification workflow (Analysis → structured Comparison → Evaluation → blind second-expert Verification) that a Person-Target Profile must pass through before it can be treated as evidence rather than a lead. Key design commitments:

- **Structured, mandatory comparison**: a checklist-style protocol across anatomical/identifying features, enforced by the system rather than left to examiner discretion.
- **Blind dual-expert review**: a second examiner repeats the process independently, without seeing the first examiner's conclusions; disagreements are flagged for resolution.
- **Immutable, provenance-linked output**: the resulting report references the originating Person-Target Profile and its evidence ledger, but carries its own, separately-signed evidentiary status — the two tiers never share legal weight.

## Evaluation Register

Unlike Papers 1 and 2, this paper's central claims are behavioral — does structured verification reduce automation bias and false-positive identification errors relative to unstructured review? This requires a human-subjects examiner study (IRB/ethics approval, a meaningful number of law-enforcement or trained examiners), comparing structured review against an unstructured baseline on metrics like decision time, error rate, and inter-examiner agreement.

## Discussion Points

- **Why structured friction works**: forcing examiners through an explicit protocol is hypothesized to override an automation-acceptance heuristic; tested via decision-time and error-rate shifts rather than assumed.
- **Deskilling concern**: does relying on an upstream tactical layer (Paper 1) to surface candidates deskill examiners who now only verify curated suggestions? This tier still requires full independent analysis, not confirmation of the upstream suggestion.
- **Where legal weight actually lives**: only this tier's signed-off output carries evidentiary weight; Paper 1's Person-Target Profile confirmations explicitly do not, and this boundary needs to stay legible to both technical and legal readers.

## Limitations & Future Work

Requires human-subjects study design and ethics approval, unlike Papers 1 and 2. Protocol and threshold choices are likely jurisdiction-specific; cross-jurisdictional validation is left to future work, as is evaluating integration overhead with the upstream lead-generation path.

## Target Journals & Keywords

Forensic-science and human-computer-interaction venues whose audience expects protocol compliance, chain-of-custody, and behavioral (bias-mitigation) evaluation rather than DSS/IS or graph-topological framing.

**Keywords**: automation bias, forensic facial comparison, dual-process theory, examiner verification, chain-of-custody, legal admissibility, target-centric intelligence.

**Avoid framing as**: face recognition accuracy benchmarking or biometric matching optimization — this is a verification-workflow and behavioral contribution, not a model contribution.
