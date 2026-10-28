# Fingerprint image processing: enhancement and finger photos

Decided 2026-10-27. Fingerprint traces only, questioned and reference alike. Two features on one
mechanism:

- **Enhancement**: the examiner's adjustments to a lift or print before it is coded: levels
  (Photoshop's *Níveis*), inverting black and white, channel choice, rotation, and resampling
  with a chosen interpolation.
- **Finger photos**: turning a photo of a finger into an image that compares like a contact
  print. The photo is either questioned material (a seized phone's picture of a hand) or a
  reference the examiner takes (an unidentified body, a field identification).

## What exists

- `ImageViewer` (`frontend/src/components/ui/image-viewer/ImageViewer.tsx`): zoom, pan, fit,
  90° rotation, and Konva's brightness/contrast/saturation filters. Its interpolation toggle only
  changes how the canvas *draws* the image on screen; it changes no pixels.
- `CodificationEditorModal` crops the evidence to the trace's box, shows it in `ImageViewer`,
  and for a fingerprint lets the examiner mark points. "Save image" uploads the canvas as PNG
  (`exportDataURL`) through `cases.SaveCodificationImage`, which stores it as a
  `codification_image` and enqueues `fingerprint:extract_codification`.
- The worker sends that image to the SourceAFIS sidecar, which extracts a template **assuming
  500 ppi** (`fingerprint.Client.Extract`; the sidecar takes `?dpi=` and defaults to 500).
- Minutiae points (`case_codification_points`) are stored in the evidence image's pixel
  coordinates.

Three problems follow for this feature:

1. **Resolution.** A scanned lift card may be 500 or 1000 ppi; a photographed lift or a
   finger photo has whatever resolution the camera and distance gave it. SourceAFIS measures
   minutia distances in millimetres through the ppi, so the wrong ppi spoils the template.
2. **No record.** The uploaded PNG is the result of browser canvas operations that are not
   written down, and canvas resampling is whatever the browser does. The image cannot be
   re-derived, and the laudo cannot say what was done to it.
3. **Coordinates.** Once the image is rotated or rescaled, points marked on it are no longer in
   evidence coordinates.

## Decisions

1. **The original is never changed; processing is a recipe.** A codification stores an ordered
   list of steps with their settings (JSON, with a version number). The server renders the
   codification image from the original evidence and the recipe, in Go, and stores the result
   with its SHA-256 as now. The same recipe on the same original always gives the same image.
   The browser shows a preview while the examiner works; what is saved and coded is the
   server's render. The steps are listed in plain Portuguese for the laudo.
2. **A fixed order of steps.** Version 1 has no free reordering. Each step is on or off, in
   this order (as implemented 2026-10-28; resampling moved up from last, see below):
   1. **Channel**: luminance (BT.601), or red, green or blue. A single channel helps when a lift
      lies on a coloured surface; green shows ridges best on skin.
   2. **Geometry and resampling, as one transform**: the trace's crop, rotation by any angle,
      horizontal mirror, and scaling to 500 ppi. The examiner chooses the interpolation:
      nearest neighbour, bilinear or bicubic (Catmull-Rom). The kernels are written out in
      `imageproc` rather than left to the browser. Before a reduction, a Gaussian anti-alias
      blur is applied, with its σ recorded.
   3. **Lighting correction** (finger photos): divide by a heavily blurred copy to remove
      uneven lighting.
   4. **Levels**: input black and white points, gamma (midtones), output black and white
      points, set against the image's histogram, as in Photoshop.
   5. **CLAHE**: local contrast normalisation.
   6. **Ridge enhancement** (finger photos): Gabor filters along the local ridge orientation,
      tuned to the measured ridge period (Hong, Wan & Jain, 1998). Areas without a ridge pattern
      become background.
   7. **Invert**: swap black and white so ridges are dark, as in a contact print.

   Resampling comes second, within the geometry, for two reasons. The image is interpolated
   once rather than twice. And the steps that work in distances (lighting correction, Gabor)
   then run at a known 500 ppi. A fixed order is easier to explain in a laudo, and is enough
   for the steps asked for. Curves, unsharp masking and background subtraction for lifts can be
   added later as further steps in the order.
3. **Resolution is a recorded fact with its source.** The codification records the source
   image's ppi and where that number came from:
   - `CALIBRATED`: the examiner draws a line along a scale in the image and types its length in
     millimetres. This is the normal case for photographed lifts and reference finger photos.
   - `FILE`: read from the file's metadata (a lift scanner's DPI). It is shown for the examiner
     to confirm.
   - `ESTIMATED`: from the ridge period, for a questioned finger photo with no scale. Adult
     inter-ridge distance is roughly 0.4–0.5 mm, so this estimate carries an error the laudo has
     to state.
   - `ASSUMED`: 500 ppi, which is today's behaviour, for images saved before this change.

   The resample step uses the source ppi to reach the output ppi, and the worker passes the
   output ppi to SourceAFIS (`Extract` gains a `dpi` argument).
4. **Geometry is affine; points stay in original coordinates.** Crop, rotation, mirror and
   scale are all affine and invertible. Points therefore stay stored against the original
   evidence, as now, and the editor maps them through the recipe's transform to draw them on the
   processed image, and back when the examiner places one. Non-affine corrections (unwrapping
   the finger's curvature) would break this, which is one reason they are left out of version 1.
5. **A finger photo is a recipe preset, not a separate pipeline.** "Finger photo" turns on:
   - the green channel;
   - flattening;
   - levels;
   - ridge enhancement;
   - invert;
   - mirror;
   - resample from the calibrated or estimated ppi.

   The examiner can change any of them. The **mirror** is needed because a camera sees the
   finger pad as it is, while a contact print is its mirror image; without it, a finger photo
   would be compared against reversed prints.
6. **A fingerprint trace records what it was taken from.** The trace gets a
   `capture_type`:
   - `LATENT`: a latent lift, the default for questioned traces;
   - `PRINT`: an inked or live-scan print sent in;
   - `FINGER_PHOTO`: a photo of the finger.

   Contactless-to-contact comparison scores lower than contact-to-contact, so the analysis and
   the laudo need to know which they are looking at, and so will any score threshold set for it.
7. **Where the work runs.** *As built in the image lab:* the server renders every change, after
   a 350 ms pause, in about a second for a fingertip, so no TypeScript copy exists yet. The split
   below remains the plan if the editor needs live sliders.
   - Levels, invert, channel and geometry are cheap and are previewed live in the browser. A
     TypeScript copy of the same formulas runs as a Konva custom filter on a downscaled working
     copy.
   - Flattening, ridge enhancement and resampling are previewed by the server on request
     ("Preview").
   - Rendering is always the server's. The Go code is a new `imageproc` package in trackid, pure
     Go with no OpenCV or cgo. It can move to trackid-vision later if another tool needs it.

## Data model (migration 012)

- `case_codifications`:
  - `recipe JSONB` and `recipe_version SMALLINT`;
  - `source_ppi NUMERIC` and `ppi_source TEXT` (`CALIBRATED` | `FILE` | `ESTIMATED` | `ASSUMED`);
  - `output_ppi SMALLINT`;
  - `rendered_at TIMESTAMPTZ`.

  The rendered image stays the codification's `case_file_id`, as now. Existing codification
  images become `ASSUMED`, 500 ppi, with no recipe (`recipe IS NULL` means "uploaded as is").
- `case_traces.capture_type TEXT`, set only on fingerprint traces (CHECK), defaulting to `LATENT`
  for questioned traces and `PRINT` for reference ones.

## API

- `POST .../traces/:traceId/codification/render` `{recipe, sourcePpi, ppiSource, outputPpi}`:
  renders, stores, links and enqueues extraction (replacing `SaveCodificationImage`'s upload
  for fingerprints). It returns the file and the recipe as stored.
- `POST .../traces/:traceId/codification/preview` `{recipe, ...}`: renders without storing, at
  reduced size, for the steps the browser does not preview.
- `GET` of the codification returns its recipe, ppi and source, so the editor reopens where
  the examiner left off.
- FACIAL codifications keep today's upload: their image is only inspected, and the embedding
  does not depend on a ppi.

## UI

- `CodificationEditorModal`, for a fingerprint trace, replaces the brightness, contrast and
  saturation sliders with a **Processing** panel listing the steps in order, each with its
  switch and settings:
  - a **histogram** with the five levels handles;
  - the **calibration tool**: draw a line on the scale, type the millimetres, and the ppi
    appears;
  - **Finger photo**: applies the preset;
  - **Original / processed**: toggles between the two;
  - **Save**: the server renders, the viewer reloads its result, and the template is extracted.
- `ImageViewer` gains what both features need, without breaking its other users:
  - a `pixelFilter` prop for a custom Konva filter;
  - rotation by any angle;
  - a measuring-line layer through `extraLayers`.

  Facial screens keep the current sliders.

## Phases

0. **Resolution plumbing.** `Client.Extract(ctx, image, dpi)`, the ppi columns, the calibration
   tool, and resampling in Go. On its own, this fixes the 500 ppi assumption for photographed
   lifts.
1. **Recipe engine** (`imageproc`): geometry, channel, levels, invert and resample, with the
   render endpoint. Tests: fixed images through each step against stored expected results, the
   same render twice giving the same SHA-256, and the affine transform's round trip for points.
2. **Editor**: the Processing panel, histogram and levels, invert, resampling choice, browser
   preview matching the server's render (within one grey level), and points drawn through the
   transform.
3. **Finger photos**:
   - flattening and CLAHE;
   - orientation field, ridge frequency and Gabor filtering;
   - the mirror;
   - the ppi estimate from ridge period;
   - the preset and `capture_type`.

   Then measure: SourceAFIS scores of finger photos against contact prints of the same fingers,
   and of different fingers, with and without each step.
4. **In the laudo**: the steps of each codification in Portuguese, when fingerprint reports
   are planned. The facial plan left them for later.

## Decided 2026-10-28

- **Output resolution: 500 ppi** (`imageproc.OutputPPI`), matching the sim, ten-print cards and
  SourceAFIS's default.
- **Test data: a manual test page for now.** The image lab, below, lets an examiner try the
  steps on any image and score the result against a reference print. Generated or public finger
  photos can come later.
- **The record follows** FISWG's *Image Processing Techniques for Facial Image Comparison*
  v1.0, §4 and §6 (from the FISWG catalog), and ASTM E2825. FISWG's guide is for faces, but its
  rules carry over:
  - the original is preserved, and processing is done on a working copy;
  - every step is listed in order with its settings;
  - the software and its version are named;
  - the processing is reproducible;
  - the result is saved losslessly;
  - the examiner uses the minimum processing needed, being aware it can introduce artifacts;
  - processing applies to the whole image, never to localized areas.

  The record therefore separates global steps from adaptive ones (lighting correction, CLAHE,
  Gabor), and warns that Gabor can join ridges. `imageproc.Describe` writes it in Portuguese.

## Status (2026-10-28)

Built:

- **The engine.** `imageproc` renders an original with a recipe (`Normalize`, `Render`),
  estimates the ridge period (`EstimateRidgePeriod`, from the radially averaged power
  spectrum) and writes the record (`Describe`). It reads the file's resolution (JFIF, PNG
  pHYs, BMP), applies the EXIF orientation as browsers do, and writes 500 ppi into the PNG.
  It is pure Go, and the same recipe always gives the same SHA-256 (tested).
- **The image lab.**
  - The page is at `/image-lab`, for any signed-in user.
  - The API is `GET /api/imageproc` plus `POST /api/imageproc/{sources,estimate,render,compare}`.
  - Originals stay in server memory for an hour; nothing is stored.
  - The page offers crop and scale-line drawing, the four resolution sources, every step in
    order, the levels histogram, and the finger-photo preset.
  - The record updates with every change, and can be copied or downloaded with the PNG and the
    recipe.
  - Comparison with SourceAFIS works when `FINGERPRINT_SIDECAR_URL` is set.
- **`fingerprint.Client.Extract` takes the dpi** (`DefaultDPI` = 500, which the worker still
  passes).

Checked on a fake finger photo:

- **The image:** made from trackid-sim's `f000_r0.png`. It was mirrored, given light ridges on
  skin colour, enlarged to 1200 ppi, rotated 14°, lit from one side and given noise.
- **Resolution:** the ridge estimate gave 1202–1204 ppi.
- **Scores:** with the finger-photo preset, SourceAFIS scored about 500 against the same
  finger and about 8 against another (threshold 40). Without the mirror, about 1 against the
  same finger.
- **Gabor:** without it, 670 against the same finger. The fake photo is clean, so Gabor adds
  nothing there; it is meant for real, noisy photos.

Not yet built: phases 0–3 inside cases.

- migration 012, the recipe and ppi on codifications, and `capture_type` on traces;
- the render endpoint for traces;
- the Processing panel in `CodificationEditorModal`;
- points drawn through the transform (`Result.Transform` is ready for it).

## Open

- **Finger photos beyond the manual lab:** extend trackid-sim to render finger photos (its
  decided rules have to be checked first), or use a public contactless dataset (most are
  licensed for research only).
- **What the institute's procedures require** to be recorded about image processing, and in
  what words. The laudo text should follow them.
- **Automatic fingertip detection** in a photo, instead of the examiner's box, is not planned;
  it could reuse the trace-detection path faces use.
