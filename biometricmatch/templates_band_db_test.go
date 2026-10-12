package biometricmatch_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// pairScores is a fake TemplateMatcher: it scores two templates by their
// bytes, in either order, and anything unlisted at 5.
type pairScores map[[2]string]float64

func (p pairScores) Match(_ context.Context, probe []byte, candidates [][]byte) ([]float64, error) {
	out := make([]float64, len(candidates))
	for i, c := range candidates {
		s, ok := p[[2]string{string(probe), string(c)}]
		if !ok {
			s, ok = p[[2]string{string(c), string(probe)}]
		}
		if !ok {
			s = 5
		}
		out[i] = s
	}
	return out, nil
}

// TestRunTemplatesBand: a template threshold version on the matcher's own,
// unbounded scale (SourceAFIS-like 40/60) applies its review band -- a
// POSITIVE at or above confirm, an INCONCLUSIVE between the cutoffs, nothing
// below -- and each decision records its cutoff and cites the version.
func TestRunTemplatesBand(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	const tt = "FINGERPRINT_SOURCEAFIS"

	set, err := biometricmatch.SetBand(ctx, sqlDB, tt, biometricmatch.Band{Review: 40, Confirm: 60}, "test band", "test")
	if err != nil {
		t.Fatalf("set a 40/60 band (match_thresholds must accept cutoffs above 1): %v", err)
	}
	v, err := biometricmatch.CurrentBand(ctx, sqlDB, tt, biometricmatch.Band{Review: 40, Confirm: 40})
	if err != nil || v.ID != set.ID {
		t.Fatalf("CurrentBand = %+v, %v; want version %d", v, err, set.ID)
	}

	// A probe and three lifts scoring 80, 50 and 30 against it, 5 against each other.
	in := cases.CaseInput{CaseID: "TEST-TBAND", CaseType: cases.CaseTypeCriminal, Modality: "FINGERPRINT",
		Evidences: []cases.EvidenceInput{{Sequence: 1}}}
	for s := int16(1); s <= 4; s++ {
		in.Evidences[0].Traces = append(in.Evidences[0].Traces, cases.TraceInput{Sequence: s, TraceType: "FINGERPRINT_LIFT"})
	}
	res, err := cases.Ingest(ctx, sqlDB, in)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"probe", "high", "mid", "low"}
	ids := make([]string, len(names))
	for i, tr := range res.Evidences[0].Traces {
		if _, err := db.New(sqlDB).UpsertBiometricTemplate(ctx, db.UpsertBiometricTemplateParams{
			BiometricfeatureID: tr.FeatureID, TemplateType: tt, Template: []byte(names[i]),
		}); err != nil {
			t.Fatal(err)
		}
		ids[i] = graph.QuestionedFeatureID(tr.TraceID)
	}
	matcher := pairScores{{"probe", "high"}: 80, {"probe", "mid"}: 50, {"probe", "low"}: 30}

	stats, err := biometricmatch.RunTemplatesBand(ctx, sqlDB, matcher, tt, v, tt)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Decisions != 2 || stats.Review != 1 {
		t.Fatalf("stats = %+v, want 2 decisions, 1 in review", stats)
	}
	rows, err := sqlDB.QueryContext(ctx, `SELECT feature_a_id, feature_b_id, decision, confidence, threshold,
		related_reference, related_reference_kind FROM biometric_decisions ORDER BY confidence DESC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []struct {
		other     string
		decision  string
		conf, thr float64
	}{{ids[1], "POSITIVE", 80, 60}, {ids[2], "INCONCLUSIVE", 50, 40}}
	i := 0
	for rows.Next() {
		var a, b, decision, ref, kind string
		var conf, thr float64
		if err := rows.Scan(&a, &b, &decision, &conf, &thr, &ref, &kind); err != nil {
			t.Fatal(err)
		}
		if i >= len(want) {
			t.Fatalf("unexpected extra decision %s|%s %s %v", a, b, decision, conf)
		}
		w := want[i]
		pair := map[string]bool{a: true, b: true}
		if !pair[ids[0]] || !pair[w.other] || decision != w.decision || conf != w.conf || thr != w.thr ||
			ref != fmt.Sprint(set.ID) || kind != "match_threshold" {
			t.Fatalf("decision %d = %s|%s %s conf %v thr %v ref %s/%s; want probe|%s %s conf %v thr %v ref %d/match_threshold",
				i, a, b, decision, conf, thr, ref, kind, w.other, w.decision, w.conf, w.thr, set.ID)
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("%d decisions, want %d", i, len(want))
	}
}
