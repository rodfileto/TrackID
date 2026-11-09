package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/fingerprint"
	"github.com/rodfileto/trackid/imageproc"
)

// The image lab (docs/fingerprint-image-processing-plan.md): an examiner uploads any image and
// tries the fingerprint processing steps on it, with the record they produce. Nothing is stored:
// originals are kept in memory for a while so each preview need not upload them again.

// ImageLab holds the lab's uploaded originals. Fingerprint, when set, compares renders with
// SourceAFIS.
type ImageLab struct {
	*labStore
	Fingerprint *fingerprint.Client
}

// NewImageLab returns an empty lab; fp may be nil (no comparison).
func NewImageLab(fp *fingerprint.Client) *ImageLab {
	return &ImageLab{labStore: newLabStore(), Fingerprint: fp}
}

type labSourceInfo struct {
	ID          string  `json:"id"`
	Filename    string  `json:"filename"`
	Format      string  `json:"format"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	SHA256      string  `json:"sha256"`
	Orientation int     `json:"orientation"`
	FilePPI     float64 `json:"filePpi"`
}

// InfoHandler says what the lab offers.
func (lab *ImageLab) InfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"outputPpi":              imageproc.OutputPPI,
			"recipeVersion":          imageproc.RecipeVersion,
			"defaultRidgeDistanceMm": imageproc.DefaultRidgeDistanceMM,
			"compare":                lab.Fingerprint != nil,
			"matchThreshold":         fingerprint.MatchThreshold,
			"software":               imageproc.Software(),
		})
	}
}

// UploadHandler takes an original (multipart "file") and keeps it for the lab's other calls.
func (lab *ImageLab) UploadHandler() gin.HandlerFunc {
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
		lab.put(&labSource{original: o, filename: filename})
		c.JSON(http.StatusOK, labSourceInfo{
			ID: o.SHA256, Filename: filename, Format: o.Format, Width: o.Width, Height: o.Height,
			SHA256: o.SHA256, Orientation: o.Orientation, FilePPI: o.FilePPI,
		})
	}
}

type labEstimateRequest struct {
	SourceID string          `json:"sourceId"`
	Crop     *imageproc.Rect `json:"crop"`
	Channel  string          `json:"channel"`
}

// EstimateHandler measures the original's ridge period, for an estimated source resolution.
func (lab *ImageLab) EstimateHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body labEstimateRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		s, ok := lab.source(c, body.SourceID)
		if !ok {
			return
		}
		period, confidence := imageproc.EstimateRidgePeriod(s.original, body.Crop, body.Channel)
		if period == 0 {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "the region is too small to measure ridges in"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"periodPx": period, "confidence": confidence})
	}
}

type labRenderRequest struct {
	SourceID string           `json:"sourceId"`
	Recipe   imageproc.Recipe `json:"recipe"`
}

type labRender struct {
	Image     string           `json:"image"` // data URL
	Width     int              `json:"width"`
	Height    int              `json:"height"`
	SHA256    string           `json:"sha256"`
	Histogram [256]int         `json:"histogram"`
	Transform imageproc.Affine `json:"transform"`
	Recipe    imageproc.Recipe `json:"recipe"`
	Record    string           `json:"record"`
	Scale     float64          `json:"scale"`
	GaborPx   float64          `json:"gaborPeriodPx"`
}

func (lab *ImageLab) render(c *gin.Context, sourceID string, recipe imageproc.Recipe) (*labSource, imageproc.Recipe, imageproc.Result, bool) {
	s, ok := lab.source(c, sourceID)
	if !ok {
		return nil, recipe, imageproc.Result{}, false
	}
	if err := recipe.Normalize(s.original.Width, s.original.Height); err != nil {
		var invalid imageproc.ValidationError
		if errors.As(err, &invalid) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read the recipe"})
		}
		return nil, recipe, imageproc.Result{}, false
	}
	res, err := imageproc.Render(s.original, recipe)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not render the image"})
		return nil, recipe, imageproc.Result{}, false
	}
	return s, recipe, res, true
}

// RenderHandler renders the recipe and returns the image with its record.
func (lab *ImageLab) RenderHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body labRenderRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		s, recipe, res, ok := lab.render(c, body.SourceID, body.Recipe)
		if !ok {
			return
		}
		record := imageproc.Describe(imageproc.Record{
			Filename: s.filename, Original: s.original, Recipe: recipe, Result: res,
			Examiner: examinerName(c), RenderedAt: time.Now(),
		})
		c.JSON(http.StatusOK, labRender{
			Image:     "data:image/png;base64," + base64.StdEncoding.EncodeToString(res.PNG),
			Width:     res.Width,
			Height:    res.Height,
			SHA256:    res.SHA256,
			Histogram: res.LevelsHistogram,
			Transform: res.Transform,
			Recipe:    recipe,
			Record:    record,
			Scale:     res.Scale,
			GaborPx:   res.GaborPeriodPX,
		})
	}
}

type labCompareRequest struct {
	SourceID        string           `json:"sourceId"`
	Recipe          imageproc.Recipe `json:"recipe"`
	ReferenceID     string           `json:"referenceId"`
	ReferenceRecipe imageproc.Recipe `json:"referenceRecipe"`
}

// CompareHandler scores the render against a reference (rendered by its own recipe, usually just
// its resolution) with SourceAFIS, both at the output resolution.
func (lab *ImageLab) CompareHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if lab.Fingerprint == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "the fingerprint matcher is not configured (FINGERPRINT_SIDECAR_URL)"})
			return
		}
		var body labCompareRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		_, _, probe, ok := lab.render(c, body.SourceID, body.Recipe)
		if !ok {
			return
		}
		_, _, reference, ok := lab.render(c, body.ReferenceID, body.ReferenceRecipe)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		templates := make([][]byte, 2)
		for i, img := range [][]byte{probe.PNG, reference.PNG} {
			t, err := lab.Fingerprint.Extract(ctx, img, imageproc.OutputPPI)
			if errors.Is(err, fingerprint.ErrUnusableImage) {
				which := []string{"processed image", "reference"}[i]
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "SourceAFIS found no fingerprint in the " + which})
				return
			}
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": "the fingerprint matcher failed: " + err.Error()})
				return
			}
			templates[i] = t
		}
		scores, err := lab.Fingerprint.Match(ctx, templates[0], [][]byte{templates[1]})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "the fingerprint matcher failed: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"score":           scores[0],
			"threshold":       fingerprint.MatchThreshold,
			"referenceWidth":  reference.Width,
			"referenceHeight": reference.Height,
			"referenceImage":  "data:image/png;base64," + base64.StdEncoding.EncodeToString(reference.PNG),
		})
	}
}

// examinerName is the signed-in user as the record names them.
func examinerName(c *gin.Context) string {
	u := currentUser(c)
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	switch {
	case name != "" && u.Username != "":
		return name + " (" + u.Username + ")"
	case name != "":
		return name
	}
	return u.Username
}
