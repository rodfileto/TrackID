package fingerprint_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rodfileto/trackid/fingerprint"
)

// sidecar connects to the sourceafis-sidecar named by FINGERPRINT_SIDECAR_URL
// (e.g. http://localhost:58090), skipping the test when it's unset or
// unreachable. These are integration tests against the real Java service
// (sourceafis-sidecar/), not a fake.
func sidecar(t *testing.T) *fingerprint.Client {
	t.Helper()
	url := os.Getenv("FINGERPRINT_SIDECAR_URL")
	if url == "" {
		t.Skip("FINGERPRINT_SIDECAR_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := fingerprint.NewClient(ctx, url)
	if err != nil {
		t.Skipf("sourceafis-sidecar unreachable: %v", err)
	}
	return client
}

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestExtractMatch checks the sidecar's scores against the fixed points
// trackid-sim's Score.java run over the same generated fingers recorded in
// its scores.csv: a rolled/rolled genuine pair (f000_r0 vs f000_r1, scored
// 545.07), a latent/rolled genuine pair (f000_l0 vs f000_r0, scored 71.57),
// and an impostor pair across fingers (f000_r0 vs f001_r0). Scores aren't
// required to be bit-identical to that run (see MatchThreshold's doc comment
// on why not), only in the same neighborhood and on the same side of
// MatchThreshold -- a regression here means the sidecar's SourceAFIS version,
// dpi handling, or template format drifted from what trackid-sim measured.
func TestExtractMatch(t *testing.T) {
	client := sidecar(t)
	ctx := context.Background()

	rolled0, err := client.Extract(ctx, testdata(t, "f000_r0.png"))
	if err != nil {
		t.Fatalf("extract f000_r0: %v", err)
	}
	rolled1, err := client.Extract(ctx, testdata(t, "f000_r1.png"))
	if err != nil {
		t.Fatalf("extract f000_r1: %v", err)
	}
	latent0, err := client.Extract(ctx, testdata(t, "f000_l0.png"))
	if err != nil {
		t.Fatalf("extract f000_l0: %v", err)
	}
	otherRolled0, err := client.Extract(ctx, testdata(t, "f001_r0.png"))
	if err != nil {
		t.Fatalf("extract f001_r0: %v", err)
	}

	scores, err := client.Match(ctx, rolled0, [][]byte{rolled1, latent0, otherRolled0})
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if len(scores) != 3 {
		t.Fatalf("got %d scores, want 3", len(scores))
	}
	genuineRR, genuineLR, impostor := scores[0], scores[1], scores[2]

	// scores.csv: genuine_rr f000_r0,f000_r1 = 545.0685.
	if genuineRR < 300 {
		t.Errorf("rolled/rolled genuine score = %.2f, want >> %v (measured 545.07)", genuineRR, fingerprint.MatchThreshold)
	}
	// scores.csv: genuine_lr f000_l0,f000_r0 = 71.5737.
	if genuineLR < fingerprint.MatchThreshold {
		t.Errorf("latent/rolled genuine score = %.2f, want >= threshold %v (measured 71.57)", genuineLR, fingerprint.MatchThreshold)
	}
	// scores.csv: impostor_rr f000_r0 vs finger 1's rolled prints, well under 40.
	if impostor >= fingerprint.MatchThreshold {
		t.Errorf("impostor score = %.2f, want < threshold %v", impostor, fingerprint.MatchThreshold)
	}
	t.Logf("genuine rolled/rolled = %.2f, genuine latent/rolled = %.2f, impostor = %.2f", genuineRR, genuineLR, impostor)
}

// TestExtractUnusableImage checks that a non-image body fails with
// ErrUnusableImage rather than a generic error, so callers (see
// fingerprint.extractAndStore) can tell "retry won't help" from a transient
// failure.
func TestExtractUnusableImage(t *testing.T) {
	client := sidecar(t)
	ctx := context.Background()
	_, err := client.Extract(ctx, []byte("not an image"))
	if !errors.Is(err, fingerprint.ErrUnusableImage) {
		t.Fatalf("err = %v, want ErrUnusableImage", err)
	}
}
