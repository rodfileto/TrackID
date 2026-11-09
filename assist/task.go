package assist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/hibiken/asynq"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/storage"
)

// TaskTypeBuildDraft is the Asynq task type cmd/worker registers a handler for (see
// HandleBuildDraft) and the API enqueues once a coordinator has staged the files of a request.
const TaskTypeBuildDraft = "assist:build_draft"

// BuildDraftPayload is TaskTypeBuildDraft's JSON payload: the staged files to read.
type BuildDraftPayload struct {
	Files []StagedFile `json:"files"`
}

// NewBuildDraftTask builds the task enqueued for a set of staged files.
func NewBuildDraftTask(files []StagedFile) (*asynq.Task, error) {
	payload, err := json.Marshal(BuildDraftPayload{Files: files})
	if err != nil {
		return nil, fmt.Errorf("assist: marshal task payload: %w", err)
	}
	return asynq.NewTask(TaskTypeBuildDraft, payload), nil
}

// DrafterFactory builds a Drafter for the occurrence types being offered. It exists so the model
// is told the same valid occurrence types Coerce checks against, fetched once per task, and so
// cmd/worker's wiring does not need to know which model client it is using.
type DrafterFactory func(occurrenceTypes []string) Drafter

// OpenAIDrafter returns the DrafterFactory for an OpenAI-compatible localhost endpoint.
func OpenAIDrafter(endpoint, model string, opts OpenAIOptions) DrafterFactory {
	return func(occurrenceTypes []string) Drafter {
		return NewOpenAIClientWithOptions(endpoint, model, occurrenceTypes, opts)
	}
}

// ResultKey names where a draft for the given staged files is stored in the bucket. It is a pure
// function of the files' content hashes, so drafting the same set twice overwrites the same
// object, and the API can compute the key it will read before the task even runs.
func ResultKey(files []StagedFile) string {
	hashes := make([]string, len(files))
	for i, f := range files {
		hashes[i] = f.SHA256
	}
	slices.Sort(hashes)
	sum := sha256.Sum256([]byte(strings.Join(hashes, "\x00")))
	return fmt.Sprintf("drafts/results/%s.json", hex.EncodeToString(sum[:]))
}

// HandleBuildDraft returns the asynq.Handler cmd/worker registers for TaskTypeBuildDraft: it
// reduces the staged files to text, asks the model for a draft, coerces it and stores the JSON
// where the API reads it. The result is a proposal only -- it writes to object storage under the
// draft prefix, never to the case tables; the coordinator's review and requests.Register do that.
func HandleBuildDraft(sqlDB db.DBTX, store *storage.Client, factory DrafterFactory) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload BuildDraftPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("%w: unmarshal task payload: %w", asynq.SkipRetry, err)
		}
		draft, err := BuildFromStaged(ctx, sqlDB, store, factory, payload.Files)
		if err != nil {
			return err
		}
		out, err := json.Marshal(draft)
		if err != nil {
			return fmt.Errorf("assist: marshal draft: %w", err)
		}
		if _, err := store.Upload(ctx, ResultKey(payload.Files), out, "application/json"); err != nil {
			return fmt.Errorf("assist: store draft: %w", err)
		}
		return nil
	}
}

// BuildFromStaged runs the whole backend path for a set of already-staged files: extract their
// text, ask the model, coerce. It is the testable seam HandleBuildDraft wraps; the caller still
// stores the result. The occurrence types offered are fetched here and shared between the model's
// prompt (through factory) and Coerce, so the two can never disagree.
func BuildFromStaged(ctx context.Context, sqlDB db.DBTX, store *storage.Client, factory DrafterFactory, files []StagedFile) (Draft, error) {
	if sqlDB == nil {
		return Draft{}, fmt.Errorf("assist: nil db")
	}
	if store == nil {
		return Draft{}, fmt.Errorf("assist: object storage is not configured")
	}
	if factory == nil {
		return Draft{}, fmt.Errorf("assist: no drafter configured")
	}
	types, err := db.New(sqlDB).ListOccurrenceTypes(ctx)
	if err != nil {
		return Draft{}, fmt.Errorf("assist: list occurrence types: %w", err)
	}
	codes := make([]string, len(types))
	for i, t := range types {
		codes[i] = t.Code
	}
	docs, err := Extract(ctx, store, files)
	if err != nil {
		return Draft{}, err
	}
	return BuildDraft(ctx, factory(codes), docs, "", codes)
}
