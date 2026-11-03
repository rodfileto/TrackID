package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/imageproc"
	"github.com/rodfileto/trackid/storage"
)

// Codification versions of a face (cases.CodificationVersion): list them, preview a recipe on the
// face, save it as a new version, choose the version the matcher uses, delete a version.

type versionTarget struct {
	caseID                  string
	evidenceID, traceID, id int64
}

// versionParams reads the case, evidence, trace and (with withID) codification of the route.
func versionParams(c *gin.Context, withID bool) (versionTarget, bool) {
	t := versionTarget{caseID: c.Param("caseId")}
	var err error
	if t.evidenceID, err = strconv.ParseInt(c.Param("evidenceId"), 10, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid evidence id"})
		return t, false
	}
	if t.traceID, err = strconv.ParseInt(c.Param("traceId"), 10, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trace id"})
		return t, false
	}
	if withID {
		if t.id, err = strconv.ParseInt(c.Param("codificationId"), 10, 64); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid codification id"})
			return t, false
		}
	}
	return t, true
}

func writeVersionError(c *gin.Context, err error) bool {
	var recipeErr imageproc.ValidationError
	var caseErr cases.ValidationError
	switch {
	case err == nil:
		return false
	case errors.Is(err, cases.ErrNotFound), errors.Is(err, cases.ErrVersionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	case errors.Is(err, cases.ErrVersionsFacialOnly), errors.Is(err, cases.ErrNotImage), errors.As(err, &recipeErr), errors.As(err, &caseErr):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case errors.Is(err, cases.ErrVersionInUse):
		c.JSON(http.StatusConflict, gin.H{"error": "the original and the version used for recognition cannot be deleted"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not process the codification versions"})
	}
	return true
}

func enqueuerOf(queue *asynq.Client) embedding.Enqueuer {
	if queue == nil {
		return nil // a nil *asynq.Client in the interface would not compare equal to nil
	}
	return queue
}

// ListCodificationVersionsHandler returns a face's codifications, the original first.
func ListCodificationVersionsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		t, ok := versionParams(c, false)
		if !ok {
			return
		}
		versions, err := cases.ListCodificationVersions(c.Request.Context(), sqlDB, t.caseID, t.evidenceID, t.traceID)
		if !writeVersionError(c, err) {
			c.JSON(http.StatusOK, gin.H{"versions": versions, "recipeVersion": imageproc.FaceRecipeVersion, "software": imageproc.FaceSoftware()})
		}
	}
}

type versionRequest struct {
	Recipe imageproc.FaceRecipe `json:"recipe"`
	Label  string               `json:"label"`
}

// PreviewCodificationVersionHandler renders a recipe on the face without saving it.
func PreviewCodificationVersionHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		t, ok := versionParams(c, false)
		if !ok {
			return
		}
		var body versionRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		r, err := cases.RenderCodificationVersion(c.Request.Context(), sqlDB, store, t.caseID, t.evidenceID, t.traceID, body.Recipe, examinerName(c))
		if !writeVersionError(c, err) {
			c.JSON(http.StatusOK, gin.H{"image": dataURL(r.Result.PNG), "width": r.Result.Width, "height": r.Result.Height,
				"sha256": r.Result.SHA256, "recipe": r.Recipe, "record": r.Record})
		}
	}
}

// CreateCodificationVersionHandler saves a recipe as a new codification of the face.
func CreateCodificationVersionHandler(sqlDB *sql.DB, store *storage.Client, queue *asynq.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		t, ok := versionParams(c, false)
		if !ok {
			return
		}
		var body versionRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		v, err := cases.CreateCodificationVersion(c.Request.Context(), sqlDB, store, enqueuerOf(queue), t.caseID, t.evidenceID, t.traceID,
			body.Recipe, body.Label, currentUser(c).ID, examinerName(c))
		if !writeVersionError(c, err) {
			c.JSON(http.StatusCreated, v)
		}
	}
}

// SetRecognitionVersionHandler makes a codification the one the matcher uses for its face.
func SetRecognitionVersionHandler(sqlDB *sql.DB, queue *asynq.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		t, ok := versionParams(c, true)
		if !ok {
			return
		}
		err := cases.SetRecognitionVersion(c.Request.Context(), sqlDB, enqueuerOf(queue), t.caseID, t.evidenceID, t.traceID, t.id)
		if !writeVersionError(c, err) {
			c.Status(http.StatusNoContent)
		}
	}
}

// DeleteCodificationVersionHandler removes a rendered version not used for recognition.
func DeleteCodificationVersionHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		t, ok := versionParams(c, true)
		if !ok {
			return
		}
		err := cases.DeleteCodificationVersion(c.Request.Context(), sqlDB, t.caseID, t.evidenceID, t.traceID, t.id)
		if !writeVersionError(c, err) {
			c.Status(http.StatusNoContent)
		}
	}
}

// CodificationVersionImageHandler returns a codification's own image.
func CodificationVersionImageHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		t, ok := versionParams(c, true)
		if !ok {
			return
		}
		data, contentType, err := cases.CodificationVersionImage(c.Request.Context(), sqlDB, store, t.caseID, t.evidenceID, t.traceID, t.id)
		if writeVersionError(c, err) {
			return
		}
		c.Header("Cache-Control", "private, max-age=3600")
		c.Data(http.StatusOK, contentType, data)
	}
}
