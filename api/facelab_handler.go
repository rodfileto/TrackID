package api

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/facecompare"
	"github.com/rodfileto/trackid/imageproc"
)

// The face lab (docs/facial-comparison-tool-plan.md): an examiner loads a questioned and a known
// face image and works through a one-to-one comparison on them, following FISWG. Nothing is stored;
// the page keeps the session and can save it as a file. Matcher scores are deliberately absent:
// the comparison is the examiner's morphological analysis.

// FaceLab holds the face lab's originals. Vision, when set, finds faces and their landmarks.
type FaceLab struct {
	*labStore
	Vision cases.FaceVision
}

// NewFaceLab returns an empty face lab; vis may be nil (landmarks placed by hand).
func NewFaceLab(vis cases.FaceVision) *FaceLab {
	return &FaceLab{labStore: newLabStore(), Vision: vis}
}

// labFace is a face the detector found: its box, score and five landmarks, in original pixels.
type labFace struct {
	Box       imageproc.Rect `json:"box"`
	Score     float32        `json:"score"`
	Landmarks [5][2]float64  `json:"landmarks"`
}

// FaceInfoHandler says what the lab offers: the feature list and stability, the analysis's
// vocabularies, the markups allowed, the opinion scale and the references.
func (lab *FaceLab) FaceInfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"components":        facecompare.Components,
			"descriptors":       facecompare.Descriptors,
			"stabilityFactors":  facecompare.StabilityFactors,
			"imageFactors":      facecompare.ImageFactors,
			"effects":           facecompare.Effects,
			"suitabilities":     facecompare.Suitabilities,
			"results":           facecompare.Results,
			"explanations":      facecompare.Explanations,
			"markupKinds":       facecompare.MarkupKinds,
			"scale":             facecompare.Scale,
			"references":        facecompare.References,
			"featureListSource": facecompare.FeatureListSource,
			"stabilitySource":   facecompare.StabilitySource,
			"recipeVersion":     imageproc.FaceRecipeVersion,
			"sessionVersion":    facecompare.SessionVersion,
			"lowIpd":            facecompare.LowIPD,
			"detection":         lab.Vision != nil,
			"software":          facecompare.Software() + "; " + imageproc.FaceSoftware(),
		})
	}
}

type faceSourceInfo struct {
	labSourceInfo
	Exif  imageproc.Exif `json:"exif"`
	Faces []labFace      `json:"faces"`
}

func faceInfo(s *labSource) faceSourceInfo {
	o := s.original
	faces := s.faces
	if faces == nil {
		faces = []labFace{}
	}
	return faceSourceInfo{
		labSourceInfo: labSourceInfo{ID: o.SHA256, Filename: s.filename, Format: o.Format, Width: o.Width, Height: o.Height,
			SHA256: o.SHA256, Orientation: o.Orientation, FilePPI: o.FilePPI},
		Exif: s.exif, Faces: faces,
	}
}

// FaceUploadHandler takes an original, reads its EXIF and finds its faces when vision is set.
func (lab *FaceLab) FaceUploadHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		data, filename, ok := readUpload(c)
		if !ok {
			return
		}
		o, err := imageproc.Decode(data)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "not an image this lab reads (JPEG, PNG, BMP, TIFF)"})
			return
		}
		s := &labSource{original: o, filename: filename, exif: imageproc.ReadExif(data)}
		if lab.Vision != nil {
			s.faces = lab.detect(data, o)
		}
		lab.put(s)
		c.JSON(http.StatusOK, faceInfo(s))
	}
}

// detect finds faces in the original as the lab sees it: when the file carries an EXIF
// orientation, on the oriented image, so the landmarks are in the same pixels as everything else.
func (lab *FaceLab) detect(data []byte, o imageproc.Original) []labFace {
	if o.Orientation >= 2 {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, o.Image, &jpeg.Options{Quality: 95}); err != nil {
			return nil
		}
		data = buf.Bytes()
	}
	dets, err := lab.Vision.DetectFaces(bytes.NewReader(data))
	if err != nil {
		log.Printf("face lab: detection failed: %v", err)
		return nil
	}
	faces := make([]labFace, 0, len(dets))
	for _, d := range dets {
		f := labFace{Box: imageproc.Rect{X1: float64(d.Box[0]), Y1: float64(d.Box[1]), X2: float64(d.Box[2]), Y2: float64(d.Box[3])}, Score: d.Score}
		for i, p := range d.Kps {
			f.Landmarks[i] = [2]float64{float64(p[0]), float64(p[1])}
		}
		faces = append(faces, f)
	}
	return faces
}

// FaceSourceHandler returns what the lab knows of a source still in memory, for a session loaded
// from a file.
func (lab *FaceLab) FaceSourceHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		s, ok := lab.source(c, c.Param("id"))
		if !ok {
			return
		}
		c.JSON(http.StatusOK, faceInfo(s))
	}
}

type faceRenderRequest struct {
	SourceID string               `json:"sourceId"`
	Recipe   imageproc.FaceRecipe `json:"recipe"`
}

// renderFace normalises the recipe against its source and renders it, answering errors itself.
func (lab *FaceLab) renderFace(c *gin.Context, sourceID string, recipe imageproc.FaceRecipe) (*labSource, imageproc.FaceRecipe, imageproc.FaceResult, bool) {
	s, ok := lab.source(c, sourceID)
	if !ok {
		return nil, recipe, imageproc.FaceResult{}, false
	}
	if err := recipe.Normalize(s.original.Width, s.original.Height); err != nil {
		var invalid imageproc.ValidationError
		if errors.As(err, &invalid) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read the recipe"})
		}
		return nil, recipe, imageproc.FaceResult{}, false
	}
	res, err := imageproc.RenderFace(s.original, recipe)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not render the image"})
		return nil, recipe, imageproc.FaceResult{}, false
	}
	return s, recipe, res, true
}

// FaceRenderHandler renders a face recipe and returns the image with its processing record.
func (lab *FaceLab) FaceRenderHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body faceRenderRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		s, recipe, res, ok := lab.renderFace(c, body.SourceID, body.Recipe)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"image":     dataURL(res.PNG),
			"width":     res.Width,
			"height":    res.Height,
			"sha256":    res.SHA256,
			"histogram": res.Histogram,
			"transform": res.Transform,
			"recipe":    recipe,
			"record": imageproc.DescribeFace(imageproc.FaceRecord{Filename: s.filename, Original: s.original, Recipe: recipe,
				Result: res, Examiner: examinerName(c), RenderedAt: time.Now()}),
		})
	}
}

type faceQualityRequest struct {
	SourceID  string                 `json:"sourceId"`
	Face      *imageproc.Rect        `json:"face"`
	Landmarks *facecompare.Landmarks `json:"landmarks"`
}

// FaceQualityHandler measures the original inside the face box: resolution, pose, sharpness and
// exposure. Numbers about the image, not the face.
func (lab *FaceLab) FaceQualityHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body faceQualityRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		s, ok := lab.source(c, body.SourceID)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, facecompare.Measure(s.original.Image, body.Face, body.Landmarks))
	}
}

type faceSideRequest struct {
	SourceID  string                 `json:"sourceId"`
	Recipe    imageproc.FaceRecipe   `json:"recipe"`
	Landmarks *facecompare.Landmarks `json:"landmarks"`
}

type faceAlignRequest struct {
	Questioned faceSideRequest         `json:"questioned"`
	Known      faceSideRequest         `json:"known"`
	Pairs      []facecompare.PointPair `json:"pairs"`
}

// transform is a side's original-to-render transform, answering errors itself.
func (lab *FaceLab) transform(c *gin.Context, side faceSideRequest) (imageproc.Affine, bool) {
	s, ok := lab.source(c, side.SourceID)
	if !ok {
		return imageproc.Affine{}, false
	}
	r := side.Recipe
	if err := r.Normalize(s.original.Width, s.original.Height); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return imageproc.Affine{}, false
	}
	return imageproc.FaceTransform(r, s.original.Width, s.original.Height), true
}

// FaceAlignHandler fits the similarity carrying the known render onto the questioned one, from the
// point pairs given or, with none, from the two images' eye centres.
func (lab *FaceLab) FaceAlignHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body faceAlignRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		qt, ok := lab.transform(c, body.Questioned)
		if !ok {
			return
		}
		kt, ok := lab.transform(c, body.Known)
		if !ok {
			return
		}
		pairs := body.Pairs
		if len(pairs) == 0 && body.Questioned.Landmarks != nil && body.Known.Landmarks != nil {
			pairs = facecompare.EyePairs(*body.Questioned.Landmarks, *body.Known.Landmarks)
		}
		sim, err := facecompare.Align(pairs, qt, kt)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"alignment": sim,
			"pairs":     pairs,
			"warnings":  nonNil(facecompare.PoseWarnings(body.Questioned.Landmarks, body.Known.Landmarks)),
		})
	}
}

type faceSessionRequest struct {
	Session facecompare.Session `json:"session"`
}

// checks are the session's warnings: what the examiner has left undone, the wording, the markups
// and, when superimposition was used, the poses.
func checks(s facecompare.Session) []facecompare.Warning {
	var out []facecompare.Warning
	add := func(code, msg, where string) {
		out = append(out, facecompare.Warning{Code: code, Message: msg, Where: where})
	}
	if !s.Questioned.Analysis.Done {
		add("ANALYSIS_PENDING", "A análise da imagem questionada não foi concluída.", "Q")
	}
	if !s.Known.Analysis.Done {
		add("ANALYSIS_PENDING", "A análise da imagem padrão não foi concluída.", "K")
	}
	for _, side := range []struct {
		code, name string
		a          facecompare.Analysis
	}{{"Q", "questionada", s.Questioned.Analysis}, {"K", "padrão", s.Known.Analysis}} {
		if side.a.Suitability == "" {
			add("SUITABILITY_MISSING", "A aptidão da imagem "+side.name+" não foi registrada.", side.code)
		} else if side.a.Suitability == "UNSUITABLE" {
			add("UNSUITABLE", "A imagem "+side.name+" foi considerada sem qualidade para comparação.", side.code)
		}
		if side.a.ManipulationSuspected {
			add("MANIPULATION", "Há suspeita de manipulação da imagem "+side.name+": encaminhe-a a perito em imagem digital ou requisite o original.", side.code)
		}
	}
	if len(s.Findings) == 0 {
		add("NO_FINDINGS", "Nenhuma característica foi registrada na comparação.", "")
	}
	if s.Opinion == nil {
		add("NO_OPINION", "A opinião não foi registrada.", "")
	}
	out = append(out, facecompare.WordingWarnings(facecompare.SessionTexts(s))...)
	out = append(out, facecompare.MarkupWarnings(s)...)
	if s.Superimposition {
		if len(s.Pairs) < 2 {
			add("SUPERIMPOSITION_UNALIGNED", "A superposição foi usada sem alinhamento registrado.", "")
		}
		out = append(out, facecompare.PoseWarnings(s.Questioned.Landmarks, s.Known.Landmarks)...)
	}
	return out
}

// FaceCheckHandler validates the session and returns its warnings and the findings' summary.
func (lab *FaceLab) FaceCheckHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body faceSessionRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		if err := body.Session.Validate(); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"warnings": nonNil(checks(body.Session)), "summary": facecompare.Summarize(body.Session)})
	}
}

// FaceSheetHandler renders both images, draws the comparison sheet (annotated and clean) and
// writes the record.
func (lab *FaceLab) FaceSheetHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body faceSessionRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		s := body.Session
		if err := s.Validate(); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		qs, qr, qres, ok := lab.renderFace(c, s.Questioned.SourceID, s.Questioned.Recipe)
		if !ok {
			return
		}
		ks, kr, kres, ok := lab.renderFace(c, s.Known.SourceID, s.Known.Recipe)
		if !ok {
			return
		}
		s.Questioned.Recipe, s.Known.Recipe = qr, kr
		qImg, err1 := decodePNG(qres.PNG)
		kImg, err2 := decodePNG(kres.PNG)
		if err1 != nil || err2 != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read the renders"})
			return
		}
		var align *facecompare.Similarity
		if len(s.Pairs) >= 2 {
			sim, err := facecompare.Align(s.Pairs, qres.Transform, kres.Transform)
			if err != nil {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
				return
			}
			align = &sim
		}
		sheet, err := facecompare.RenderSheet(facecompare.SheetImage{Image: qImg, Transform: qres.Transform},
			facecompare.SheetImage{Image: kImg, Transform: kres.Transform}, align, s.Markups)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not draw the sheet"})
			return
		}
		now := time.Now()
		examiner := examinerName(c)
		side := func(src *labSource, side facecompare.Side, r imageproc.FaceRecipe, res imageproc.FaceResult) facecompare.SideRecord {
			return facecompare.SideRecord{
				Filename: src.filename, Original: src.original, Exif: src.exif,
				Processing: imageproc.DescribeFace(imageproc.FaceRecord{Filename: src.filename, Original: src.original, Recipe: r, Result: res, Examiner: examiner, RenderedAt: now}),
				Metrics:    facecompare.Measure(src.original.Image, side.Face, side.Landmarks),
			}
		}
		warnings := checks(s)
		record := facecompare.Describe(facecompare.RecordInput{
			Session: s, Questioned: side(qs, s.Questioned, qr, qres), Known: side(ks, s.Known, kr, kres),
			Alignment: align, Sheet: &sheet, Warnings: warnings, Examiner: examiner, At: now,
		})
		c.JSON(http.StatusOK, gin.H{
			"annotated":       dataURL(sheet.Annotated),
			"annotatedSha256": sheet.AnnotatedSHA256,
			"clean":           dataURL(sheet.Clean),
			"cleanSha256":     sheet.CleanSHA256,
			"record":          record,
			"warnings":        nonNil(warnings),
			"summary":         facecompare.Summarize(s),
		})
	}
}

func dataURL(png []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

func decodePNG(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

func nonNil(w []facecompare.Warning) []facecompare.Warning {
	if w == nil {
		return []facecompare.Warning{}
	}
	return w
}
