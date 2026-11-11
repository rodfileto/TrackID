package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/collections"
)

// writeCollectionError writes the response for a collections error and reports whether there was one.
func writeCollectionError(context *gin.Context, err error) bool {
	var validation collections.ValidationError
	switch {
	case err == nil:
		return false
	case errors.Is(err, collections.ErrCaseNotFound):
		context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.As(err, &validation):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not record the material"})
	}
	return true
}

// RequireSceneCollection guards registering a scene examined on call: coordinators and fingerprint
// examiners (docs/scene-collection-plan.md).
func RequireSceneCollection(sqlDB *sql.DB) gin.HandlerFunc {
	return requireCapability(sqlDB, func(u auth.UserProfile) bool {
		return u.Coordinates() || u.Examines(auth.ModalityFingerprint)
	}, "coordinator or fingerprint examiner access required")
}

type registerCollectionRequest struct {
	OccurrenceID     *int64                     `json:"occurrenceId"`
	Occurrence       *createOccurrenceRequest   `json:"occurrence"`
	Description      string                     `json:"description"`
	CollectedFrom    time.Time                  `json:"collectedFrom"`
	CollectedTo      *time.Time                 `json:"collectedTo"`
	CollectedByID    int64                      `json:"collectedById"` // the collector; the caller when omitted
	NothingCollected bool                       `json:"nothingCollected"`
	Notes            string                     `json:"notes"`
	ReportMode       string                     `json:"reportMode"`
	Vestiges         []collections.VestigeInput `json:"vestiges"`
	CalledAt         *time.Time                 `json:"calledAt"`
	OtherOfficers    string                     `json:"otherOfficers"`
	Team             []int64                    `json:"team"`
	SceneNoteID      int64                      `json:"sceneNoteId"` // the field note it is registered from
}

// RegisterCollectionHandler records a scene examined on call, opening its fingerprint case
// (collections.Register). An examiner registers their own collection; a coordinator may register
// one for any fingerprint examiner.
func RegisterCollectionHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var body registerCollectionRequest
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid collection"})
			return
		}
		user := currentUser(context)
		collector := body.CollectedByID
		if collector == 0 {
			collector = user.ID
		}
		if collector != user.ID && !user.Coordinates() {
			context.JSON(http.StatusForbidden, gin.H{"error": "only a coordinator registers a collection for someone else"})
			return
		}
		profile, err := auth.NewRepository(sqlDB).FindByID(context.Request.Context(), collector)
		if err != nil || !profile.Examines(auth.ModalityFingerprint) {
			context.JSON(http.StatusBadRequest, gin.H{"error": "the collector must be an active fingerprint examiner"})
			return
		}
		in := collections.RegisterInput{OccurrenceID: body.OccurrenceID, Description: body.Description,
			CollectedFrom: body.CollectedFrom, CollectedTo: body.CollectedTo, CollectedBy: collector,
			NothingCollected: body.NothingCollected, Notes: body.Notes, ReportMode: body.ReportMode, Vestiges: body.Vestiges,
			CalledAt: body.CalledAt, OtherOfficers: body.OtherOfficers, Team: body.Team, SceneNoteID: body.SceneNoteID}
		if body.Occurrence != nil {
			o := body.Occurrence.input()
			in.Occurrence = &o
		}
		r, err := collections.Register(context.Request.Context(), sqlDB, in, user.ID, time.Now())
		if !writeCollectionError(context, err) {
			context.JSON(http.StatusCreated, r)
		}
	}
}

// ListCaseOriginsHandler returns a case's report mode, scene collections and vestiges with their seals.
func ListCaseOriginsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := collections.List(context.Request.Context(), sqlDB, context.Param("caseId"))
		if !writeCollectionError(context, err) {
			context.JSON(http.StatusOK, list)
		}
	}
}

// ListAwaitingCollectionsHandler returns the collections still awaiting the request that formalizes them.
func ListAwaitingCollectionsHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		list, err := collections.ListAwaiting(context.Request.Context(), sqlDB)
		if !writeCollectionError(context, err) {
			context.JSON(http.StatusOK, list)
		}
	}
}

// AddVestigeHandler records a physical item of the case, received with one of its requests or
// collected in one of its collections.
func AddVestigeHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var body collections.AddVestigeInput
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid vestige"})
			return
		}
		v, err := collections.AddVestige(context.Request.Context(), sqlDB, context.Param("caseId"), body, currentUser(context).ID, time.Now())
		if !writeCollectionError(context, err) {
			context.JSON(http.StatusCreated, v)
		}
	}
}

func vestigeID(context *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("vestigeId"), 10, 64)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid vestige id"})
		return 0, false
	}
	return id, true
}

// BreakSealHandler records that the current user opened a vestige's seal.
func BreakSealHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, ok := vestigeID(context)
		if !ok {
			return
		}
		err := collections.BreakSeal(context.Request.Context(), sqlDB, context.Param("caseId"), id, currentUser(context).ID, time.Now())
		if !writeCollectionError(context, err) {
			context.Status(http.StatusNoContent)
		}
	}
}

// ResealHandler records that the current user sealed a vestige under a new number.
func ResealHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		id, ok := vestigeID(context)
		if !ok {
			return
		}
		var body struct {
			Number string `json:"number"`
		}
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid seal"})
			return
		}
		err := collections.Reseal(context.Request.Context(), sqlDB, context.Param("caseId"), id, body.Number, currentUser(context).ID, time.Now())
		if !writeCollectionError(context, err) {
			context.Status(http.StatusNoContent)
		}
	}
}

// SetReportModeHandler changes whether a case's analyses are reported together or separately,
// until its first laudo is issued.
func SetReportModeHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		var body struct {
			ReportMode string `json:"reportMode"`
		}
		if err := context.ShouldBindJSON(&body); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid report mode"})
			return
		}
		err := collections.SetReportMode(context.Request.Context(), sqlDB, context.Param("caseId"), body.ReportMode)
		if !writeCollectionError(context, err) {
			context.Status(http.StatusNoContent)
		}
	}
}

// formID reads an optional id from a multipart form field; ok is false after a 400 for a bad one.
func formID(context *gin.Context, field string) (int64, bool) {
	v := strings.TrimSpace(context.PostForm(field))
	if v == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + field})
		return 0, false
	}
	return id, true
}

// materialOrigin reads where uploaded material came from: base's request or collection when the
// route names one, else the form's requestId or collectionId; and the form's vestigeId. It checks
// the origin is the case's (collections.CheckOrigin) before anything is stored; ok is false after
// the response was written.
func materialOrigin(context *gin.Context, sqlDB *sql.DB, caseID string, base collections.Origin) (collections.Origin, bool) {
	o := base
	var ok bool
	if o.RequestID == 0 && o.CollectionID == 0 && o.AnalysisID == 0 {
		if o.RequestID, ok = formID(context, "requestId"); !ok {
			return o, false
		}
		if o.CollectionID, ok = formID(context, "collectionId"); !ok {
			return o, false
		}
		if o.AnalysisID, ok = formID(context, "analysisId"); !ok {
			return o, false
		}
	}
	if o.VestigeID, ok = formID(context, "vestigeId"); !ok {
		return o, false
	}
	if writeCollectionError(context, collections.CheckOrigin(context.Request.Context(), sqlDB, caseID, o)) {
		return o, false
	}
	return o, true
}
