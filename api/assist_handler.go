package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rodfileto/trackid/assist"
)

// The assist lab: upload the files a request would send in, see the text extracted from each, and
// ask the local model to draft the request/occurrence/case. Nothing is stored and nothing is
// written to the database -- it exists to try the drafting pipeline by hand and inspect its
// output, not to register anything. The real flow (staging, the worker task, review, then
// requests.Register) is separate and untouched.

// assistLabMaxBytes bounds one uploaded file, a guard rather than a feature.
const assistLabMaxBytes = 40 << 20

// AssistLab drafts a registration from uploaded files, in memory. drafter may be nil (no local
// model configured): extraction still works, drafting answers 503.
type AssistLab struct {
	drafter assist.Drafter
	model   string
}

// NewAssistLab returns an assist lab. drafter is nil when no local model is configured.
func NewAssistLab(drafter assist.Drafter, model string) *AssistLab {
	return &AssistLab{drafter: drafter, model: model}
}

// InfoHandler says whether drafting is configured and with what model.
func (lab *AssistLab) InfoHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"enabled": lab.drafter != nil, "model": lab.model})
	}
}

// ExtractHandler reads one uploaded file and returns its document: the extracted text for a PDF or
// plain text, empty for an image or scan.
func (lab *AssistLab) ExtractHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		data, filename, ok := readUploadedFile(c)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, assist.ExtractDocument(filename, data))
	}
}

// assistLabMaxInstructions bounds the operator's typed instructions, in bytes.
const assistLabMaxInstructions = 64 << 10

type draftRequest struct {
	Documents []assist.Document `json:"documents"`
	// Instructions is the operator's free text: directions to the model, or data the files lack.
	Instructions string `json:"instructions"`
}

// DraftHandler asks the model for a draft from the documents sent (already extracted, possibly
// edited) and the operator's instructions -- either may be empty, not both -- and returns the coerced Draft -- the exact proposal a coordinator would review.
func (lab *AssistLab) DraftHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if lab.drafter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no local model is configured (ASSIST_ENDPOINT/ASSIST_MODEL)"})
			return
		}
		var body draftRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}
		if len(body.Instructions) > assistLabMaxInstructions {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the instructions are longer than 64 KB"})
			return
		}
		if len(body.Documents) == 0 && strings.TrimSpace(body.Instructions) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "add a file or type instructions"})
			return
		}
		draft, err := assist.BuildDraft(c.Request.Context(), lab.drafter, body.Documents, body.Instructions, assist.DefaultOccurrenceTypes)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, draft)
	}
}

// readUploadedFile reads the multipart "file", answering the error itself when it cannot.
func readUploadedFile(c *gin.Context) (data []byte, filename string, ok bool) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return nil, "", false
	}
	if file.Size > assistLabMaxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the file is larger than 40 MB"})
		return nil, "", false
	}
	opened, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
		return nil, "", false
	}
	defer opened.Close()
	data, err = io.ReadAll(opened)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
		return nil, "", false
	}
	return data, file.Filename, true
}
