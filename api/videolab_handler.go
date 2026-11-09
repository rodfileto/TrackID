package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/video"
)

// The video lab (video.Lab): send any video, have trackid-vision find the faces in it, grouped by
// person with the best face of each marked, and step through it on the page. Nothing is stored;
// the video stays in a temporary file on the server only so full frames can be extracted.

// videoLabMaxBytes bounds an uploaded video, as for a case's videos.
const videoLabMaxBytes = video.MaxBytes

// VideoLab serves the video lab over lab (which may have no tracker: uploads then answer 503).
type VideoLab struct {
	lab *video.Lab
}

// NewVideoLab returns the video lab's handlers over lab.
func NewVideoLab(lab *video.Lab) *VideoLab { return &VideoLab{lab: lab} }

// InfoHandler says whether the lab can process videos, with its defaults and bounds.
func (h *VideoLab) InfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"enabled":         h.lab.Enabled(),
			"defaultInterval": video.LabDefaultInterval,
			"minInterval":     video.LabMinInterval,
			"maxInterval":     video.LabMaxInterval,
			"defaultMinBlur":  video.LabDefaultMinBlur,
			"maxBytes":        videoLabMaxBytes,
			"software":        video.Software(),
		})
	}
}

// UploadHandler takes a video (multipart field "file", optional "intervalSeconds" and "minBlur")
// and starts processing it; the page then polls JobHandler.
func (h *VideoLab) UploadHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.lab.Enabled() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": video.ErrLabNoTracker.Error()})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, videoLabMaxBytes+1<<20)
		reader, err := c.Request.MultipartReader()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "send the video as multipart form data"})
			return
		}
		// Fields before the file are read as options; the file is streamed to disk, not buffered.
		var opts video.LabOptions
		for {
			part, err := reader.NextPart()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "no video was sent (field \"file\")"})
				return
			}
			switch part.FormName() {
			case "intervalSeconds", "minBlur":
				raw, _ := io.ReadAll(io.LimitReader(part, 32))
				v, perr := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
				if perr != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + part.FormName()})
					return
				}
				if part.FormName() == "intervalSeconds" {
					opts.IntervalSeconds = v
				} else {
					opts.MinBlur = v
				}
			case "file":
				job, err := h.lab.Add(part.FileName(), part, videoLabMaxBytes, opts)
				switch {
				case errors.Is(err, video.ErrNotVideo):
					c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
				case errors.Is(err, video.ErrLabBusy):
					c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
				case err != nil:
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				default:
					c.JSON(http.StatusAccepted, job)
				}
				return
			}
		}
	}
}

func (h *VideoLab) jobError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, video.ErrLabJobNotFound):
		c.JSON(http.StatusGone, gin.H{"error": err.Error(), "code": "source_expired"})
	case errors.Is(err, video.ErrOutOfRange):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// JobHandler returns a video's processing: its progress, then the persons found.
func (h *VideoLab) JobHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		job, err := h.lab.Get(c.Param("id"))
		if err != nil {
			h.jobError(c, err)
			return
		}
		c.JSON(http.StatusOK, job)
	}
}

// DeleteHandler stops a video's processing and deletes it from the server.
func (h *VideoLab) DeleteHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := h.lab.Remove(c.Param("id")); err != nil {
			h.jobError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// FrameHandler returns the full frame at ?ms= as a PNG, extracted by ffmpeg from the original.
func (h *VideoLab) FrameHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ms, err := strconv.ParseInt(c.Query("ms"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ms must be a time in milliseconds"})
			return
		}
		png, err := h.lab.Frame(c.Request.Context(), c.Param("id"), ms)
		if err != nil {
			h.jobError(c, err)
			return
		}
		c.Header("Cache-Control", "private, max-age=3600")
		c.Data(http.StatusOK, "image/png", png)
	}
}
