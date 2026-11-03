package assist

import (
	"context"
	"testing"

	"github.com/rodfileto/trackid/requests"
)

type fakeDrafter struct {
	draft Draft
	err   error
}

func (f fakeDrafter) Draft(ctx context.Context, docs []Document, instructions string) (Draft, error) {
	return f.draft, f.err
}

func TestBuildDraftCoerces(t *testing.T) {
	d := fakeDrafter{draft: Draft{Request: requests.RegisterInput{
		Input:    requests.Input{Authority: " 37ª DP ", ReceivedOn: "2026-01-01"},
		CaseType: "criminal",
	}}}
	got, err := BuildDraft(context.Background(), d, nil, "", []string{"LETHAL"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Request.Input.Authority != "37ª DP" || got.Request.CaseType != "CRIMINAL" {
		t.Errorf("draft not coerced: %+v", got.Request)
	}
}

func TestBuildDraftNoDrafter(t *testing.T) {
	if _, err := BuildDraft(context.Background(), nil, nil, "", nil); err == nil {
		t.Error("expected error with no drafter")
	}
}
