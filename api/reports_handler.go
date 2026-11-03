package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/examination"
	"github.com/rodfileto/trackid/reports"
	"github.com/rodfileto/trackid/storage"
)

// maxDraftBytes bounds an uploaded draft.
const maxDraftBytes = 64 << 20

// reportService is the reports' dependencies for one request. store may be nil (not configured).
func reportService(sqlDB *sql.DB, store *storage.Client, renderer reports.Renderer) reports.Service {
	s := reports.Service{DB: sqlDB, Renderer: renderer}
	if store != nil {
		s.Store = store
		s.Images = func(ctx context.Context, featureID string) (reports.Image, error) {
			img, err := examination.FeatureImage(ctx, sqlDB, store, featureID)
			return reports.Image{Data: img.Data, ContentType: img.ContentType}, err
		}
	}
	return s
}

// writeReportError writes the response for a reports error and reports whether there was one.
func writeReportError(context *gin.Context, err error) bool {
	var missing reports.MissingSectionsError
	var validation reports.ValidationError
	switch {
	case err == nil:
		return false
	case errors.Is(err, analyses.ErrCannotDecide):
		context.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, analyses.ErrAnalysisNotFound):
		context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.As(err, &missing):
		context.JSON(http.StatusConflict, gin.H{"error": err.Error(), "sections": missing.Sections})
	case errors.Is(err, analyses.ErrNotDraft), errors.Is(err, analyses.ErrNotReportStage), errors.Is(err, reports.ErrStale),
		errors.Is(err, reports.ErrNotIssued):
		context.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, reports.ErrLockedChanged), errors.Is(err, reports.ErrRegionsChanged), errors.Is(err, reports.ErrNotDocx),
		errors.As(err, &validation):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, reports.ErrNoRenderer), errors.Is(err, reports.ErrNoStore):
		context.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, reports.ErrNoDocument):
		context.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not process the report"})
	}
	return true
}

// GetReportHandler returns an analysis's report: its executive summary (preliminary while a
// draft), its draft's latest version, documents and deliveries. It is hidden while the viewer
// works the analysis blind.
func GetReportHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		o, err := reports.Service{DB: sqlDB}.Get(context.Request.Context(), target.ID)
		if !writeReportError(context, err) {
			context.JSON(http.StatusOK, o)
		}
	}
}

// ReportDraftHandler returns the draft .docx for its examiner to edit, generating the first one.
// X-Report-Version and X-Report-Stale describe it.
func ReportDraftHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		v, data, err := reportService(sqlDB, store, nil).Draft(context.Request.Context(), currentUser(context), target.ID)
		if writeReportError(context, err) {
			return
		}
		context.Header("X-Report-Version", strconv.Itoa(int(v.Version)))
		context.Header("X-Report-Stale", strconv.FormatBool(v.Stale))
		context.Header("Cache-Control", "private, no-store")
		context.Data(http.StatusOK, reports.DocxType, data)
	}
}

// SaveReportDraftHandler stores the examiner's edited .docx (the request body) as a new version.
func SaveReportDraftHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		data, err := io.ReadAll(io.LimitReader(context.Request.Body, maxDraftBytes+1))
		if err != nil || len(data) == 0 || len(data) > maxDraftBytes {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid document"})
			return
		}
		v, err := reportService(sqlDB, store, nil).Save(context.Request.Context(), currentUser(context), target.ID, data)
		if !writeReportError(context, err) {
			context.JSON(http.StatusOK, v)
		}
	}
}

// RefreshReportDraftHandler regenerates the draft from the data, keeping the examiner's text.
func RefreshReportDraftHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		v, err := reportService(sqlDB, store, nil).Refresh(context.Request.Context(), currentUser(context), target.ID)
		if !writeReportError(context, err) {
			context.JSON(http.StatusOK, v)
		}
	}
}

// IssueReportHandler issues an analysis's report (reports.Service.Issue): only its first examiner,
// once every pair is settled, and only from a draft that matches the data.
func IssueReportHandler(sqlDB *sql.DB, store *storage.Client, renderer reports.Renderer) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		report, err := reportService(sqlDB, store, renderer).Issue(context.Request.Context(), currentUser(context), target.ID)
		if !writeReportError(context, err) {
			context.JSON(http.StatusOK, report)
		}
	}
}

// ReportDocumentHandler downloads an issued report's PDF (?format=pdf, the default) or .docx.
func ReportDocumentHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		data, name, contentType, err := reportService(sqlDB, store, nil).Document(context.Request.Context(), target.ID, context.DefaultQuery("format", "pdf"))
		if writeReportError(context, err) {
			return
		}
		context.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
		context.Data(http.StatusOK, contentType, data)
	}
}

// DeliverReportHandler records that an issued report was sent: by a coordinator, or by whoever
// issued it.
func DeliverReportHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		target, ok := caseAnalysis(context, sqlDB)
		if !ok {
			return
		}
		user := currentUser(context)
		issuer, err := reports.IssuedBy(context.Request.Context(), sqlDB, target.ID)
		if writeReportError(context, err) {
			return
		}
		if !user.Coordinates() && issuer != user.Username {
			context.JSON(http.StatusForbidden, gin.H{"error": "only a coordinator or the report's issuer records its delivery"})
			return
		}
		var in reports.Delivery
		if err := context.ShouldBindJSON(&in); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid delivery"})
			return
		}
		if writeReportError(context, reports.Service{DB: sqlDB}.Deliver(context.Request.Context(), user, target.ID, in)) {
			return
		}
		o, err := reports.Service{DB: sqlDB}.Get(context.Request.Context(), target.ID)
		if !writeReportError(context, err) {
			context.JSON(http.StatusCreated, o)
		}
	}
}
