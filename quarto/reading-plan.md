---
title: "TrackID — Reading Plan"
subtitle: "What to read for each chapter of the book, from theory to the applied system"
author: Rodrigo Fileto Cuerci Maciel
lang: pt-BR
---

## How to use this plan

- This plan follows the chapters of `book-plan.md`. Read the sources for a chapter just before
  writing it, so the notes turn straight into text. Part I carries most of the reading. Parts II
  and III need less, and their sources are applied ones.
- **Status** is either the key in `book/references.bib` or **add**, meaning the source is not in
  the bibliography yet. Any **add** item has to be checked first: it was listed from memory, so
  confirm its year, venue and DOI (e.g. in `openalex-lake`) before it goes into the `.bib`.
- **Optional** marks background that strengthens a chapter but isn't required for its argument.
- Out of scope here: epistemology of science (trackid-methods); Design Science, simulation-study
  design and score-to-LR calibration (the papers, see `research-plan.md`).

## Parte I — Fundamentos

### 1. Identificação de pessoas e seus silos

| Source | What to extract | Status |
|---|---|---|
| NAS (2009) | How forensic identification services are organized; the fragmentation critique | `nas2009` |
| Ribaux, Walsh & Margot (2006) | Case-by-case service vs the intelligence view; why traces end up in silos | `ribaux2006` |
| Jain, Ross & Nandakumar, *Introduction to Biometrics* (chs. 1, fingerprint, face) | Enrollment vs query, 1:1 vs 1:N, AFIS/ABIS in general terms | add |

### 2. Inteligência forense

| Source | What to extract | Status |
|---|---|---|
| Ribaux & Margot (1999) | Traces as information beyond the court case; early linkage systems | `ribaux1999` |
| Ribaux, Walsh & Margot (2006) | Forensic intelligence defined; the intelligence cycle | `ribaux2006` |
| Ribaux, *Police scientifique* (2014) | The full framework; series and unknown subjects | `ribaux2014` |
| Rossy et al. (2013) | Integrating forensic information into intelligence practice | `rossy2013` |
| Morelato et al. (2014) | The forensic intelligence framework; evidence for trial vs information for intelligence | `morelato2014` |
| Roux et al., forensic science as a security science | *Optional:* the wider rationale for the discipline | add |

### 3. ACE-V e a comparação forense

| Source | What to extract | Status |
|---|---|---|
| Ashbaugh (1999) | ACE-V's origin and stages in friction-ridge identification | `ashbaugh1999` |
| Huber | The earlier formulation of the method that Ashbaugh builds on | add |
| FISWG one-to-many review v1.0 (2026) | One-to-many review vs one-to-one comparison; the conclusion scale | `fiswg2026review` |
| FISWG facial comparison overview | Extending ACE-V to faces | `fiswgoverview` |
| FISWG one-to-one comparison guideline | Morphological analysis, verification | add |
| Dror & Charlton (2006) | Contextual bias in examiners' decisions | `dror2006` |
| Dror et al. (2015), Linear Sequential Unmasking | Why sequencing and blindness matter | `dror2015` |
| NAS (2009); PCAST (2016) | The critiques: validity, error rates, subjectivity | `nas2009`, `pcast2016` |
| Ulery et al. (2011, 2012) | Black-box accuracy; repeatability and reproducibility of decisions | `ulery2011`, `ulery2012` |
| Saks & Koehler (2005); Cole (2009); Stoney (1991) | *Optional:* the individualization debate, as background to the conclusion scales | add |

### 4. Inteligência coletiva

| Source | What to extract | Status |
|---|---|---|
| Surowiecki (2004) | Independence, diversity, aggregation; when crowds fail | `surowiecki2004` |
| Hong & Page (2004) | Diversity can outperform ability | `hong2004` |
| Woolley et al. (2010) | A collective intelligence factor in groups | `woolley2010` |
| Malone & Bernstein (2015) | Collective intelligence in organizations; design dimensions | `malone2015` |
| Phillips et al. (2018) | Examiners, super-recognizers and algorithms fused beat any of them alone | `phillips2018` |
| White et al. | Combining face examiners' judgements; *check the year: the plan says 2015, the bib says 2014* | `white2014` |

### 5. Humanos e máquinas na identificação

| Source | What to extract | Status |
|---|---|---|
| Parasuraman & Manzey (2010) | Automation bias and complacency | `parasuraman2010` |
| Grother et al. (2019), NISTIR 8280 | Demographic differentials; thresholds as policy | `grother2019demographic` |
| NIST FRVT/FRTE 1:N reports | What a score means; operating points; the review band | add |
| ISO/IEC 19795-1 | Vocabulary and method for biometric performance testing | add |
| Phillips et al. (2018) | Human-machine fusion (picked up again from ch. 4) | `phillips2018` |

## Parte II — O sistema

The main source for these chapters is TrackID itself (`MODEL.md`, migrations, screens). The
readings below cover the general ideas each chapter implements.

### 6. O modelo de dados

| Source | What to extract | Status |
|---|---|---|
| `MODEL.md` §1–5 | The model as built | internal |
| W3C PROV-O | *Optional:* vocabulary for "who decided what, from which inputs" | add |
| CASE/UCO ontology | *Optional:* how digital forensics models cases and provenance | add |

### 7. Do escore à decisão

| Source | What to extract | Status |
|---|---|---|
| ISO/IEC 19795-1; NIST FRTE | Thresholds, FMR/FNMR, and setting a review band | add |
| Grother et al. (2019) | Why one threshold does not fit all groups | `grother2019demographic` |

### 8. ACE-V no TrackID

| Source | What to extract | Status |
|---|---|---|
| Ashbaugh (1999); FISWG | Mapping each stage of the system to ACE-V | see ch. 3 |
| Dror et al. (2015) | Justifying blind verification and stage order | `dror2015` |

### 9. Distribuição e papéis

| Source | What to extract | Status |
|---|---|---|
| Surowiecki (2004); Malone & Bernstein (2015) | Independence as a design requirement for distribution | see ch. 4 |
| Kassin, Dror & Kukucka (2013) | *Optional:* the forensic confirmation bias, as the reason not to give two steps to one person | add |

### 10. Da decisão à inteligência

| Source | What to extract | Status |
|---|---|---|
| Fellegi & Sunter (1969) | *Optional:* record linkage theory behind links → clusters | add |
| Christen, *Data Matching* | *Optional:* entity resolution, transitive closure, merge/split errors | add |
| Ribaux (2014) | Series and unknown subjects (closing the loop to ch. 2) | `ribaux2014` |

### 11. Auditoria, privacidade e limites

| Source | What to extract | Status |
|---|---|---|
| LGPD (Lei 13.709/2018) | Sensitive personal data (biometrics); the public-security exception (art. 4º, III) | `lgpd2018` |
| ANPD guidance on biometric data | *Optional:* current regulatory reading | add |
| Append-only logs / event sourcing (e.g. Fowler) | *Optional:* the engineering pattern behind the decision log | add |

## Parte III — O exercício

### 12. Um mundo sintético com verdade conhecida

| Source | What to extract | Status |
|---|---|---|
| trackid-sim `docs/PLAN.md`, `OCCURRENCE.md`, `LICENSES.md` | How the world is generated | internal |
| Rubin (1993); Drechsler, *Synthetic Datasets for Statistical Disclosure Control* | *Optional:* what synthetic data can and cannot show | add |

### 13. Roteiro da aula

No readings needed. The sources are the trackid screens and the trackid-sim `users` command.

### 14. Avaliando as decisões

| Source | What to extract | Status |
|---|---|---|
| Amigó et al. (2009) | B-cubed and other cluster metrics for scoring merges and splits | add |
| Phillips et al. (2018); Hong & Page (2004) | Collective vs individual accuracy (the empirical side of ch. 4) | see ch. 4 |

### 15. Discussão e próximos passos

No new readings. Draw on `research-plan.md` and the roadmap.

## Order

1. Chapters 2 → 3 → 4 → 5, the theoretical core; most of the existing bibliography is here.
2. Chapter 1, written last in Part I, once you know what the later chapters set up.
3. Chapters 6–11, alongside the code.
4. Chapters 12–15, alongside trackid-sim.
