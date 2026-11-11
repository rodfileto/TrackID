package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/geocode"
	"github.com/rodfileto/trackid/scenenotes"
	"github.com/rodfileto/trackid/storage"
)

// Notes written at a scene and synced from the device (docs/scene-examination-plan.md). Every route
// is for those who register scene collections (RequireSceneCollection).

func sceneNotes(sqlDB *sql.DB, store *storage.Client, geocoder geocode.Geocoder) (scenenotes.Service, bool) {
	if sqlDB == nil || store == nil {
		return scenenotes.Service{}, false
	}
	return scenenotes.Service{DB: sqlDB, Store: store, Geocoder: geocoder}, true
}

func writeSceneNoteError(context *gin.Context, err error) bool {
	var validation scenenotes.ValidationError
	switch {
	case err == nil:
		return false
	case errors.Is(err, scenenotes.ErrNotFound):
		context.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
	case errors.Is(err, scenenotes.ErrForbidden):
		context.JSON(http.StatusForbidden, gin.H{"error": "the note is someone else's"})
	case errors.Is(err, scenenotes.ErrConverted):
		context.JSON(http.StatusConflict, gin.H{"error": "the note is already a collection"})
	case errors.As(err, &validation):
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		context.JSON(http.StatusInternalServerError, gin.H{"error": "could not keep the note"})
	}
	return true
}

func sceneNoteID(context *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(context.Param(name), 10, 64)
	if err != nil || id <= 0 {
		context.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return id, true
}

// SyncSceneNoteHandler keeps a note sent from a device, new or again (scenenotes.Sync).
func SyncSceneNoteHandler(sqlDB *sql.DB, store *storage.Client, geocoder geocode.Geocoder) gin.HandlerFunc {
	return func(context *gin.Context) {
		service, ok := sceneNotes(sqlDB, store, geocoder)
		if !ok {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database or storage is not configured"})
			return
		}
		var in scenenotes.Input
		if err := context.ShouldBindJSON(&in); err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "invalid note"})
			return
		}
		note, err := service.Sync(context.Request.Context(), currentUser(context).ID, in, time.Now())
		if !writeSceneNoteError(context, err) {
			context.JSON(http.StatusOK, note)
		}
	}
}

// ListSceneNotesHandler lists the caller's notes; ?everyone=true everyone's for a coordinator, and
// ?converted=true also those that already became a collection.
func ListSceneNotesHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		service, ok := sceneNotes(sqlDB, store, nil)
		if !ok {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database or storage is not configured"})
			return
		}
		notes, err := service.List(context.Request.Context(), currentUser(context),
			context.Query("everyone") == "true", context.Query("converted") == "true")
		if !writeSceneNoteError(context, err) {
			context.JSON(http.StatusOK, gin.H{"items": notes})
		}
	}
}

// GetSceneNoteHandler returns one note with its audios.
func GetSceneNoteHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		service, ok := sceneNotes(sqlDB, store, nil)
		id, valid := sceneNoteID(context, "noteId")
		if !ok || !valid {
			if !ok {
				context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database or storage is not configured"})
			}
			return
		}
		note, err := service.Get(context.Request.Context(), currentUser(context), id)
		if !writeSceneNoteError(context, err) {
			context.JSON(http.StatusOK, note)
		}
	}
}

// DiscardSceneNoteHandler removes a note that is not a collection yet.
func DiscardSceneNoteHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		service, ok := sceneNotes(sqlDB, store, nil)
		id, valid := sceneNoteID(context, "noteId")
		if !ok || !valid {
			if !ok {
				context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database or storage is not configured"})
			}
			return
		}
		if !writeSceneNoteError(context, service.Discard(context.Request.Context(), currentUser(context), id)) {
			context.Status(http.StatusNoContent)
		}
	}
}

// AddSceneNoteAudioHandler keeps an audio recorded with a note: multipart, with the file in "file",
// its device id in "clientId", and optionally "title" and "recordedAt" (RFC 3339).
func AddSceneNoteAudioHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		service, ok := sceneNotes(sqlDB, store, nil)
		id, valid := sceneNoteID(context, "noteId")
		if !ok || !valid {
			if !ok {
				context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database or storage is not configured"})
			}
			return
		}
		header, err := context.FormFile("file")
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}
		if header.Size > scenenotes.MaxAudioBytes {
			context.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the audio is too large"})
			return
		}
		file, err := header.Open()
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
			return
		}
		defer file.Close()
		content, err := io.ReadAll(io.LimitReader(file, scenenotes.MaxAudioBytes+1))
		if err != nil {
			context.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
			return
		}
		in := scenenotes.AudioInput{ClientID: context.PostForm("clientId"), Title: context.PostForm("title"),
			ContentType: header.Header.Get("Content-Type"), Content: content}
		if at := context.PostForm("recordedAt"); at != "" {
			recorded, err := time.Parse(time.RFC3339, at)
			if err != nil {
				context.JSON(http.StatusBadRequest, gin.H{"error": "recordedAt must be RFC 3339"})
				return
			}
			in.RecordedAt = &recorded
		}
		audio, err := service.AddAudio(context.Request.Context(), currentUser(context), id, in)
		if !writeSceneNoteError(context, err) {
			context.JSON(http.StatusOK, audio)
		}
	}
}

// SceneNoteAudioHandler returns an audio note's file.
func SceneNoteAudioHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		service, ok := sceneNotes(sqlDB, store, nil)
		id, valid := sceneNoteID(context, "audioId")
		if !ok || !valid {
			if !ok {
				context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database or storage is not configured"})
			}
			return
		}
		content, contentType, err := service.AudioContent(context.Request.Context(), currentUser(context), id)
		if !writeSceneNoteError(context, err) {
			context.Header("Cache-Control", "private, no-store")
			context.Data(http.StatusOK, contentType, content)
		}
	}
}

// CollectionNoteFileHandler returns an audio note of one of a case's collections.
func CollectionNoteFileHandler(sqlDB *sql.DB, store *storage.Client) gin.HandlerFunc {
	return func(context *gin.Context) {
		if sqlDB == nil || store == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database or storage is not configured"})
			return
		}
		id, valid := sceneNoteID(context, "fileId")
		if !valid {
			return
		}
		file, err := db.New(sqlDB).GetCollectionNoteFile(context.Request.Context(), db.GetCollectionNoteFileParams{ID: id, CaseID: context.Param("caseId")})
		if errors.Is(err, sql.ErrNoRows) {
			context.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not read the file"})
			return
		}
		content, err := store.Download(context.Request.Context(), file.StorageRef.String)
		if err != nil {
			context.JSON(http.StatusBadGateway, gin.H{"error": "could not read the file"})
			return
		}
		context.Header("Cache-Control", "private, no-store")
		context.Data(http.StatusOK, file.ContentType.String, content)
	}
}

// SceneTeamHandler lists who can be named in a note's or a collection's team: the active
// fingerprint examiners.
func SceneTeamHandler(sqlDB *sql.DB) gin.HandlerFunc {
	return func(context *gin.Context) {
		if sqlDB == nil {
			context.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not configured"})
			return
		}
		users, err := db.New(sqlDB).ListFingerprintExaminers(context.Request.Context())
		if err != nil {
			context.JSON(http.StatusInternalServerError, gin.H{"error": "could not list the examiners"})
			return
		}
		items := make([]scenenotes.Member, 0, len(users))
		for _, u := range users {
			items = append(items, scenenotes.Member{ID: u.ID, Username: u.Username, Name: strings.TrimSpace(u.FirstName + " " + u.LastName)})
		}
		context.JSON(http.StatusOK, gin.H{"items": items})
	}
}
