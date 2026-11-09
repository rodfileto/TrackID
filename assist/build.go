package assist

import (
	"context"
	"fmt"
)

// BuildDraft asks the model for a draft from the extracted documents and the operator's
// instructions (possibly empty), and always runs the result
// through Coerce, so whatever the model returned is safe to show for review. occurrenceTypes are
// the codes of the occurrence_types table, checked against the model's choices.
func BuildDraft(ctx context.Context, d Drafter, docs []Document, instructions string, occurrenceTypes []string) (Draft, error) {
	if d == nil {
		return Draft{}, fmt.Errorf("assist: no drafter configured")
	}
	draft, err := d.Draft(ctx, docs, instructions)
	if err != nil {
		return Draft{}, err
	}
	return Coerce(draft, occurrenceTypes), nil
}
