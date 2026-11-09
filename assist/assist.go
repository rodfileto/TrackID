// Package assist turns the files a request sends in into a proposed registration: an authority's
// request, the occurrence it cites and the cases it opens, for a coordinator to review before
// requests.Register runs. Nothing here writes to the case tables; it only produces a
// requests.RegisterInput (the shape POST /api/requests already accepts). The model must run
// locally -- nothing leaves the institute -- and the human review is the caller's job, not this
// package's: a draft is a suggestion, never a record.
//
// Drafter is the boundary, the model behind it an implementation detail (see OpenAIClient, a
// client for any OpenAI-compatible localhost endpoint such as Strata or Ollama). The flow a
// caller runs is: stage the uploaded files (UploadDraftFile), reduce them to text (Extract), ask
// the model (BuildDraft, which always coerces the result through Coerce), and show the draft to a
// coordinator. The coordinator's submission goes through the normal requests.Register path, so
// the existing validation is the last word regardless of what the model produced.
package assist

import (
	"context"

	"github.com/rodfileto/trackid/requests"
)

// Drafter proposes a requests.RegisterInput from the text of a request's files and the
// operator's own instructions. A nil Drafter
// means drafting is disabled, the same convention as api.Dependencies.Geocoder and
// ReportRenderer: the caller returns 503 rather than failing.
type Drafter interface {
	// Draft reads the documents and returns a proposed registration. instructions is free text
	// the operator typed -- directions ("only the second file is the ofício") or data the files do
	// not carry ("received on 2026-10-30") -- and may be empty. The result is never
	// authoritative: BuildDraft coerces it and the caller shows it for review.
	Draft(ctx context.Context, docs []Document, instructions string) (Draft, error)
}

// Document is one file reduced to what a model reads: its name and its text. Text is empty for a
// file with no extractable text (a scanned page, an image) -- Extract never invents text, and
// OCR is deliberately not part of it, since it is the expensive step a deploying institute opts
// into separately.
type Document struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Text   string `json:"text"`
}

// StagedFile is one uploaded file held in object storage under a draft prefix, before any case
// exists to hold it. UploadDraftFile writes it; the worker's Extract downloads it back.
type StagedFile struct {
	SHA256      string `json:"sha256"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	StorageRef  string `json:"storageRef"`
}

// Draft is a proposed registration plus what the model is unsure about. Request is the shape
// POST /api/requests takes; Unsure names the fields (as JSON paths) the model guessed, so a
// review screen can highlight exactly those. A field the model did not find is left empty -- it
// never invents one. Reasoning is the model's own note on what it found and did not, for the
// review screen, and is not stored.
type Draft struct {
	Request   requests.RegisterInput `json:"request"`
	Unsure    []string               `json:"unsure,omitempty"`
	Reasoning string                 `json:"reasoning,omitempty"`
	// Usage is what producing the draft cost, when the model server reports it; for tuning, not
	// for review.
	Usage *Usage `json:"usage,omitempty"`
}

// Usage is one model call's cost: tokens read and written, and the wall time it took.
// ReasoningTokens counts hidden "thinking" tokens when the server reports them separately.
type Usage struct {
	PromptTokens     int   `json:"promptTokens"`
	CompletionTokens int   `json:"completionTokens"`
	ReasoningTokens  int   `json:"reasoningTokens,omitempty"`
	DurationMS       int64 `json:"durationMs"`
}
