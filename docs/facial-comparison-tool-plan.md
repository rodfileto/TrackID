# Facial comparison tool: side by side, markups and the feature list

Decided and built 2026-10-30. A **face lab** page (`/face-lab`), like the image lab: an
examiner loads two face images, questioned and known, and works through ACE-V on them. That means
processing each image, aligning the two, marking features with markups, and recording each
feature's comparison. The page also writes the record and exports the comparison sheet. Nothing is
stored, and nothing is wired into cases yet. The Go functions behind the page are the ones the
case worksheet will call later.

## What exists

- **The facial comparison as an analysis type** (`FACIAL_COMPARISON`): pairs, worksheet, blind
  rounds and a graded conclusion (`examination`, `AnalysisWorkspace`). The worksheet shows the two
  faces side by side, takes a decision, a conclusion and free notes, and does nothing more. It has
  no markups, no feature list and no per-image analysis.
- **Suitability** per questioned trace (`examination/suitability.go`): `SUITABLE`, `LEAD_ONLY` or
  `UNSUITABLE`.
- **trackid-vision**: detection gives a box, a score and **5 landmarks** (eye centres, nose tip,
  mouth corners) in original pixels. `vision.Face` also has the embedding, and
  `cases.CompareFaces` returns the cosine of two codifications.
- **`imageproc`** and the image lab (`api/imageproc_handler.go`, `pages/ImageLab.tsx`):
  - a recipe rendered in Go and reproducible to the SHA-256;
  - originals kept in memory for an hour;
  - the record in Portuguese (`Describe`).

  The engine is **grey only** (`plane`) and works in ppi, both of which fit fingerprints, not
  faces.
- **`ImageViewer`** (Konva): zoom, pan, fit, 90° rotation, `extraLayers` for annotation layers,
  and `keepView`. It cannot share its view with a second viewer and has no overlay of two images.
- **trackid-sim** generates faces (`tools/facegen`), which gives test images: several images of the
  same synthetic person, and images of different people.

## What FISWG says the tool must do

From the local catalog (`../fiswg-catalog`, fetched 2026-10-27). These are the documents that bear
on a one-to-one comparison tool, with what each one requires of it.

| Document | What it requires of the tool |
|---|---|
| *Facial Comparison Overview and Methodology Guidelines* v2.1 (2026) | **Morphological analysis is the method** (§6.2). The tool compares components and their characteristics. It should not offer classification schemes like "round face" or "Roman nose" (§6.2.3). **Superimposition** is allowed only alongside morphological analysis, never on its own (§6.3.1). It needs the same viewpoint, and alignment may only scale and rotate, with nothing that skews shapes (§6.3.2). Image quality limits it (§6.3.3). **Photo-anthropometry shall not be used** (§6.4.1, §6.4.3). The tool therefore has no measuring tools: no distances, ratios or angles between landmarks. |
| *Guideline for the Use of ACE-V in One-to-One Examinations* v1.0 (2023) | **Analysis:** the questioned image is analysed before the known one (§5.1.1). The analysis covers imaging factors, capture timeframe, alterations, subject factors and metadata (§5.1.2), and ends in a suitability call (§5.1.3). **Comparison:** side by side, by the feature list, with stability in mind. It notes similarities and dissimilarities, and which dissimilarities subject variance or imaging explains (§5.2.1). It may produce annotated visual aids (§5.2.2). **Evaluation:** weighs quality, quantity, specificity, expected variation and persistence (§5.3). No absolute certainty is allowed, and the words *individualize, identifies, identification, includes, inclusion, excludes, exclusion* are forbidden in an opinion (§5.3.2.2). **Verification:** blind (§5.4), which the case rounds already do. |
| *Feature List for Morphological Analysis* v2.0 (ASTM E3149) | The **19 components**, top of the face to the bottom: skin, face/head outline, face/head composition, hair, forehead, eyebrows, eyes, cheeks, nose, ears, mouth, chin/jawline, neck, facial hair, facial lines, scars, facial marks, alterations, other. Each is split into component characteristics and descriptors, many with right and left sides. "Distance" means relative size, never a value (§3.4). This list is the worksheet's structure. |
| *Physical Stability of Facial Features of Adults* v2.0 (2021) | For each characteristic, a **High/Medium/Low** stability under six factors: expression, short-term time, long-term time, marked weight change, health, and intentional alteration. The tables run pp. 6–15. Example: intercanthal distance is H under all six, the iris is L under expression. The worksheet shows these next to each characteristic, and the examiner says which factors apply to the pair. |
| *Best Practices for Markups & Annotations* v1.0 (2021) | **Allowed markups** (§4.2): circles or boxes for marks and scars; hash marks or brackets for relative distance and groupings; arrows, numbers or dots for specific features. **Forbidden** (§4.3): drawing or tracing on features, text or text boxes over the head, face or neck, markups that hide the feature they point at, and cluttered or overlapping markups. Every annotated image goes with its **non-annotated** copy (§1.2). Use anatomical terms from E3149 (§5.2), and never words like *unique, significant, same, identical, exact* for a feature (§5.2.1). |
| *Image Processing Techniques for Facial Image Comparison* v1.0 (2022) | Work on a copy, use the minimum processing, and apply it to the **whole image only** (§4.3–4.5). Allowed techniques (§5.1): brightness/contrast, crop, rotation, colour balance, levels, curves, greyscale or channel extraction, and mirroring. Record every step, the software and its version, and make the processing reproducible. Save losslessly (§6). This is what `imageproc` already does for fingerprints. |
| *Image Factors to Consider* v1.0 (2021) | The **analysis checklist**: artifacts (blooming, banding, ringing, blocking, aliasing), distortion (aspect ratio, barrel, perspective at under 2 m, rolling shutter, interlacing, **mirroring**), lighting, near infrared, noise, obstructions, pose (pitch, roll and yaw; **roll does not change the features**, pitch and yaw do), sharpness and resolution. **Interpupillary distance (IPD) in pixels** helps judge effective resolution (§13.1), and enlarging an image adds no detail. |
| *Minimum Guidelines for Documentation* v2.0 (2022) | What the record holds. For analysis (§6): suitability, the factors, processed images and their steps. For comparison (§7): the E3149 features, imaging effects and stability, and a visual example. For evaluation (§8): the scale, the criteria and the reasoning. Also the software, its version and configuration (§10.1.5). |
| *Visual Image Manipulation Detection* v1.0 (2024) | The examiner looks for signs of manipulation: copy-move, smoothing, morphing and beauty filters. A suspicion is referred to a digital image expert, or the original image is asked for (§4.4). The tool only records the suspicion; it does no detection. |

The other documents are about FR systems, training, capture or printing, and add nothing to this
tool. *Non-Frontal Facial Image Capture* and *3D Pose Correction* concern pose. They support
warning on a pose mismatch, but not correcting pose in v1. The *Conflict Resolution* document has
no text in the catalog. It may matter for the third-examiner round, not for this tool.

## Decided 2026-10-30

- **The comparison's conclusion scale is FISWG's**: FISWG's ACE-V guideline (§5.3.2) and its
  quality management draft (§14.7.1) point to OSAC 2022-S-0001, whose five categories are the
  strength of support for a common source against different sources. It replaces the seven-level
  working scale everywhere, in cases and laudos too (below).
- **No matcher score** in the comparison, not even behind a toggle (decision 14 below).
- **The lab records an opinion** on that scale, with its reasoning (decision 10).
- **FISWG's descriptive text is not copied**: the code keeps names and H/M/L ratings only, and the
  record cites every document used, by title, version and section.

## Decisions

1. **A face lab page, separate from the image lab.**
   - It sits at `/face-lab`, for any signed-in user, under the image lab in the sidebar.
   - It uses the same "nothing is stored" model as the image lab.
   - The lab's in-memory source store, now inside `ImageLab`, moves to a small shared type that
     both labs use.
   - The fingerprint lab stays as it is.
2. **Two panels: questioned on the left, known on the right. The questioned image is analysed
   first** (ACE-V §5.1.1).
   - The known panel stays covered until the questioned image's analysis is marked done.
   - A switch turns this off in the lab. In cases it will be the rule.
3. **Processing reuses `imageproc`, with colour added.**
   - `imageproc` gains an RGB plane and a **face recipe**: crop, rotation by any angle, mirror,
     resampling with a chosen interpolation, brightness/contrast, levels, curves, colour balance,
     and greyscale or channel.
   - That is FISWG's list, all of it global.
   - No CLAHE, flattening or Gabor for faces: they are adaptive or local, which §4.5 rules out.
   - Faces have no ppi. Size is set by a target IPD in pixels instead, or left as it is.
   - `Describe` writes the face record in Portuguese, as it does for fingerprints.
4. **Landmarks come from the detector, and the examiner can correct them.**
   - On upload, trackid-vision detects the faces, when it is configured. The examiner picks one
     face, and its 5 landmarks are shown and can be dragged.
   - Without vision, or for a profile, the examiner places them.
   - Landmarks are tools for alignment, never something compared.
5. **Alignment is a similarity transform: rotation, uniform scale and translation, nothing else.**
   - It is fitted from 2 or more pairs of corresponding points: the eye centres by default, or
     any points the examiner pairs, such as for a profile. With 3 or more pairs it is a least
     squares fit.
   - Because the transform has no shear and no non-uniform scale, it keeps shapes (§6.3.2).
   - **It is a view transform**: the two panels show the faces at the same scale and roll, but
     no pixels change.
   - Only the exported sheet resamples, and its record says so. When a face is enlarged, the
     record states that enlarging adds no detail (Image Factors §13).
6. **Views can be linked.** Once the images are aligned, zooming or panning one panel moves the
   other to the same region of the face.
   - **Guide lines**: horizontal or vertical lines drawn across both panels at once, for example
     through the eye line or the nasal base. They show relative position without measuring
     anything.
7. **Superimposition is an auxiliary mode, with three views:**
   - **opacity overlay**;
   - **wipe**: a slider splitting the two images;
   - **flicker**: alternating between them.

   The mode needs both images aligned. It warns when the rough poses differ (the next decision)
   and when either IPD is low. The record says it was used "em conjunto com a análise
   morfológica" (§6.3.1). There is no difference or blend mode that would produce a new image.
8. **No measurements.** The tool shows no distances, ratios or angles between facial points
   (§6.4).
   - The only numbers are **about the image**: IPD in pixels (resolution), roll, a rough
     yaw/pitch estimate (pose), sharpness and clipped exposure. The UI labels them as such.
   - Brackets and hash marks are markups, with no value attached.
9. **Markups are limited to the kinds FISWG allows:**
   - ellipse;
   - box;
   - bracket or hash mark;
   - arrow;
   - numbered dot.

   - **No freehand pen, and no text on the image.** Each markup has a **number**, and its
     description goes in the findings table outside the image.
   - A markup placed on one side can be placed on the other with the same number, so the pair
     corresponds.
   - Each image gets one style (colour and stroke) that is used throughout.
   - The tool **warns** when markups overlap, or when a markup's interior hides most of the feature
     it marks; for example, it suggests an arrow instead of a filled box.
   - Markups are stored in **original image coordinates**, like the fingerprint points, and drawn
     through the processing and alignment transforms. Both are affine and invertible, so they
     stay correct when either changes.
10. **Findings follow the E3149 feature list.** A finding is:

    ```
    component → characteristic (→ descriptor) [right/left side]
    observation in Q · observation in K
    result: SIMILAR | DISSIMILAR | NOT_COMPARABLE (not visible in one or both)
    for DISSIMILAR: explained by one or more of subject variance (expression, pose,
                    weight, age, health, alteration) and imaging conditions, or
                    unexplained (which excludes the others)
    markups: the numbers that illustrate it
    ```

    - The list is shown as an accordion of the 19 components. Each characteristic carries its
      **stability chips** (H/M/L under each factor), and the examiner ticks which factors apply to
      this pair, such as an expression change or years apart.
    - A summary counts the findings by result and by stability, which helps the evaluation step
      but **is not the opinion**.
    - The opinion is a level of the scale with its reasoning. In cases it will be the worksheet's
      existing decision and conclusion, on the same scale.
11. **The analysis panel, for each image:**
    - the source file, its SHA-256 and dimensions;
    - EXIF capture date, camera and software, when present;
    - the Image Factors checklist, each factor with a note;
    - pose and IPD from the landmarks;
    - mirrored? (a flag, Image Factors §6.1.10);
    - manipulation suspected? (a flag, with the note to refer it);
    - a suitability call with its reason.

    Suitability uses the trace values already defined (`SUITABLE`, `LEAD_ONLY`, `UNSUITABLE`), so
    wiring it into cases changes nothing.
12. **A wording check, which warns but never blocks.** It looks for the terms FISWG rules out,
    in Portuguese and in English:
    - for features: *único, significativo, mesmo, idêntico, exato*;
    - in opinions: *individualiza, identifica, identificação, inclui, exclui, exclusão, com
      exclusão de todos os outros*.

    It runs on the observations and notes. *Mesmo* gives false positives ("mesmo assim"), which is
    why it only warns.
13. **The server renders the export.** The comparison sheet is a PNG with these parts:
    - the two images, processed and aligned, annotated and **un-annotated**;
    - the markup numbers;
    - the findings table, as text;
    - the record.

    It is drawn in Go (`golang.org/x/image` already provides a font), so the image that later goes
    into the laudo never comes from a browser canvas, as with the laudo PDF. The page can also
    download the session as JSON, with the recipes, landmarks, markups and findings, and load it
    back.
14. **No matcher score.** The comparison is the examiner's morphological analysis; the lab
    neither shows nor records the embedding's similarity. Vision is used only to find the face and
    its landmarks.

## Backend

**`imageproc` (extended).**
- An RGB plane alongside the grey `plane`. Geometry and resampling are written once and run on
  either.
- A `FaceRecipe`, with its own version, validation and `RenderFace(original, recipe)`. It returns
  the PNG and the affine `Transform`.
- `DescribeFace(record)`: the processing record in Portuguese.
- Tests:
  - each step against stored expected images;
  - the same render twice gives the same SHA-256;
  - the transform's round trip for points.

**`facecompare` (new package; pure Go except for detection, which goes through `cases.FaceVision`).**
- `features.go`: the E3149 components, characteristics and descriptors, with stable codes and
  labels in English and Portuguese.
- `stability.go`: the H/M/L tables, transcribed from FISWG's PDF with the page of each.
- `align.go`: fits a similarity transform from pairs of points and composes it with each image's
  processing transform.
- `quality.go`:
  - IPD in pixels;
  - roll from the eyes;
  - a rough yaw and pitch from the nose and mouth landmarks relative to the eyes (an estimate,
    labelled as one);
  - sharpness, as the variance of the Laplacian over the face box;
  - clipped highlights and shadows.
- `markup.go`: the markup types, checking their kind and geometry, and the overlap warnings.
- `findings.go`: the finding type, checked against the feature list, with the summary counts.
- `wording.go`: the wording check, returning each match and its position.
- `sheet.go`: renders the comparison sheet.
- `describe.go`: the comparison record in Portuguese. Sections follow FISWG's documentation guide:
  - análise de cada imagem;
  - processamento;
  - método: análise morfológica; superposição, when used, as an auxiliary; and the statement that
    no fotoantropometria was used;
  - características comparadas;
  - software and version.

**API** (`api/facelab_handler.go`, signed-in users, nothing stored):

| Route | Does |
|---|---|
| `GET /api/facelab` | the feature list, the stability tables, the vision status, the software version |
| `POST /api/facelab/sources` | upload. Returns the SHA-256, the size, the EXIF fields, and the detected faces (box, score, landmarks), when vision is configured |
| `POST /api/facelab/render` | `{sourceId, recipe}` → the processed PNG, the transform and the processing record |
| `POST /api/facelab/quality` | `{sourceId, landmarks, box}` → IPD, pose and the image metrics |
| `POST /api/facelab/align` | `{left, right, pointPairs}` → the transform from right to left, with the residual and the pose warnings |
| `POST /api/facelab/check` | `{texts}` → the wording warnings |
| `GET /api/facelab/sources/:id` | a source still in memory, for a session loaded from a file |
| `POST /api/facelab/sheet` | the session → the sheet PNGs (annotated and not), the warnings, the summary and the full record |

## UI

- **`pages/FaceLab.tsx`**: two `ImageViewer`s with a shared toolbar:
  - select;
  - the markup tools;
  - landmarks;
  - guide lines;
  - align;
  - link views;
  - side by side / overlay / wipe / flicker.

  Below or beside the viewers are tabs:
  - **Analysis**: Q and K;
  - **Processing**: Q and K, a lighter version of the image lab's panel;
  - **Comparison**: the feature list accordion and the findings;
  - **Summary**: the counts and the warnings;
  - **Record**: the text and the exports.
- **`components/facelab/`** (as built):
  - `FaceStage`: a set of layers and tool handlers inside `ImageViewer`, which supplies the stage,
    zoom, pan, fit, toolbar and zoom indicator. It shows one face side by side, or both in
    overlay, wipe and flicker. It draws markups, landmarks, point pairs, guides and the crop box,
    all kept in original pixels and drawn through the processing and alignment transforms.
  - `ImageViewer` gained, without changing its other users:
    - an optional controlled view (`view` + `onViewChange`), which links the two panels;
    - `imageTransform`, which places an aligned known image in the questioned image's frame;
    - `cursor`;
    - a guard so that dragging a shape in `extraLayers` does not pan.

    It runs with `showFilters` off: its browser-side brightness, contrast and saturation are not
    recorded.
  - `AnalysisPanel`, `ProcessingPanel`, `FindingsPanel` (with the stability chips) and
    `EvaluationPanel`.
  - `controls.tsx`, `style.ts` and `geometry.ts`.
- i18n in `en.json` and `pt-BR.json`. The FISWG terms are in English with a Portuguese label, and
  the record is in Portuguese.

## Phases

1. **Engine and API.**
   - The colour path and face recipe in `imageproc`.
   - `facecompare`: the feature list, stability, alignment, quality, wording and the record.
   - The handlers, with the shared lab store.
   - Tests on trackid-sim face images.
2. **Side by side.**
   - The page with two viewers.
   - Upload and face picking, landmarks, and processing.
   - Alignment, linked views and guide lines.
   - The Analysis tab, with questioned-first.
3. **Markups and findings.**
   - The markup tools and numbering.
   - The feature list with stability.
   - Findings linked to markups, and the wording check.
   - The summary.
4. **Superimposition and export.**
   - Overlay, wipe and flicker, with the pose warning.
   - The server-rendered sheet, the record, and the session JSON (save and load).
5. **Later, in cases (not now).**
   - The face codification takes a recipe, as in the fingerprint plan.
   - The comparison worksheet stores findings and markups **per pair, per examiner and per
     step**, so the verifier does a blind ACE-V of their own.
   - The laudo's section 6 ("Exames") takes each pair's sheet and findings.
   - The facial review gets a lighter version.

Check: a pair from trackid-sim of the same person and a pair of different people, taken through
the whole page. Then a browser check of linked views, of markups surviving a change of rotation
or crop, and of the exported sheet.

## The conclusion scale (migration 015)

- `cluster.Conclusion` is `STRONG_SUPPORT_COMMON`, `SUPPORT_COMMON`, `INCONCLUSIVE`,
  `SUPPORT_DIFFERENT`, `STRONG_SUPPORT_DIFFERENT`. Common source maps to POSITIVE, different
  sources to NEGATIVE, so the rounds logic is unchanged. OSAC subdivides the support categories
  only where research shows examiners apply finer ones reliably, so the five are kept.
- Migration 015 rewrites stored conclusions and identification levels: `IDENTIFICATION` and
  `STRONG_SUPPORT` become `STRONG_SUPPORT_COMMON`; `MODERATE_SUPPORT` and `LIMITED_SUPPORT`
  become `SUPPORT_COMMON`; `LIMITED_EXCLUSION` becomes `SUPPORT_DIFFERENT`; `EXCLUSION` becomes
  `STRONG_SUPPORT_DIFFERENT`. Its Down maps them back to `STRONG_SUPPORT`, `MODERATE_SUPPORT`,
  `LIMITED_EXCLUSION` and `EXCLUSION`; the finer distinctions are lost. It also adds CHECKs
  naming the scale's values, because `conclusion_decision` returns NULL for an unknown value and
  a CHECK passes on NULL.
- The default identification level (the lowest that counts as identified in TrackID) is
  `STRONG_SUPPORT_COMMON`.
- The laudo words it after OSAC. It has the scale table, a paragraph citing OSAC and FISWG, and
  "Nível mínimo para registro do vínculo no sistema". The facial results read "Origem comum
  favorecida" and "Origens distintas favorecidas" rather than "Identificado" and "Excluído".
  The fingerprint laudo is unchanged. The wording lives in `facecompare.Scale`, which `reports`
  reads.

## Status (2026-10-30)

Built:

- **`imageproc`**:
  - the face recipe (`FaceRecipe`, `RenderFace`, `FaceTransform`), in colour, with only FISWG's
    global steps;
  - its record (`DescribeFace`);
  - an EXIF reader (`ReadExif`: capture date, camera, software).

  The geometry is shared with the fingerprint recipe (`affineGeometry`).
- **`facecompare`**:
  - the feature list (19 components) and the stability ratings, by page;
  - the session, findings and summary;
  - the similarity fit (`Fit`, `Align`), pose and image metrics;
  - markup rules and warnings, and the wording check;
  - the server-drawn sheet;
  - the Portuguese record with its references;
  - the scale's wording.
- **API**: `api/facelab_handler.go`. The labs' in-memory store moved to `api/labstore.go`, which
  both labs share.
- **Page**: `/face-lab` (`pages/FaceLab.tsx`, `components/facelab/`), in the sidebar under the
  image lab.
  - Questioned and known stages, with questioned-first.
  - Detection, with editable landmarks.
  - Processing with its record.
  - Alignment by the eyes or by point pairs, with linked views.
  - Overlay, wipe and flicker.
  - Guides.
  - The five markup kinds, numbered and paired, movable and renumberable.
  - The feature list with stability chips and findings.
  - The evaluation, with the opinion on the scale.
  - The sheet and the record.
  - Saving and loading the session.

Checked:

- **Go tests**:
  - each face step, reproducibility, and the transform's round trip;
  - EXIF;
  - the feature list and stability data;
  - the similarity fit, pose and metrics;
  - wording, markups, validation and summary;
  - the sheet's pixels and the record;
  - the API end to end with a fake detector;
  - the laudo's wording;
  - all DB-backed tests;
  - migration 015 down and up, with the CHECK refusing an old value.
- **Browser** (headless Chromium, on a scratch API and database beside the dev stack), with
  trackid-sim's `face_000`: a degraded 1920 × 1080 frame as questioned (IPD 38 px, warned) and the
  portrait as known.
  - Detection on both images.
  - The questioned image cropped and levelled, and the known one scaled to its IPD.
  - Alignment by the eyes: scale 1.001, RMS 0 px.
  - Ellipse, dot and bracket markups paired by number, and a guide across both panels.
  - Overlay and wipe.
  - Findings on the nasal tip and the right eyebrow, and an opinion.
  - The sheet and the record. The record warned about "idêntica".
  - The session saved, then reloaded in a new page in Portuguese.
  - No console errors.

## Open

- **Wiring into cases** (phase 5): face codifications with a recipe, findings and markups per
  pair, examiner and step, and the sheet in the laudo's section 6.
- **The opinion scale's criteria**: OSAC 2022-S-0001 asks each laboratory to define what is needed
  to reach each category, and to state that no validation study of the scale exists while none
  does. Both are the institute's to write.
- **Pose correction** (FISWG's 3D pose correction) and **aspect ratio correction**: both are
  left out of v1. Aspect ratio correction is not a similarity transform, and pose correction
  synthesises pixels.
- **Profile and ear comparisons:** the 5 landmarks are frontal. For a profile, the examiner pairs
  points by hand, which decision 5 already allows.
- **Video frames**: a frame from `video` could be loaded straight into the lab once the lab is
  wired to cases.
- **The low-IPD warning** (40 px) is TrackID's working value, not a FISWG figure.
