package biometricmatch_test

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// unitVec is a 512-d unit vector with the given components on its first axes.
func unitVec(parts ...float64) string {
	v := make([]string, 512)
	for i := range v {
		v[i] = "0"
		if i < len(parts) {
			v[i] = fmt.Sprintf("%.9f", parts[i])
		}
	}
	return "[" + strings.Join(v, ",") + "]"
}

// TestRunBand: a threshold version applies its review band -- a POSITIVE above confirm, an
// INCONCLUSIVE between the cutoffs, nothing below -- and each decision records the cutoff it was
// classified against, in similarity units, and cites the version.
func TestRunBand(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	const et = "FACE_ARCFACE_512"

	fallback := biometricmatch.Band{Review: 0.6, Confirm: 0.6}
	v, err := biometricmatch.CurrentBand(ctx, sqlDB, et, fallback)
	if err != nil || v.ID != 0 || v.Band != fallback || v.Source != "default" {
		t.Fatalf("CurrentBand with no version = %+v, %v; want the fallback", v, err)
	}
	if _, err := biometricmatch.SetBand(ctx, sqlDB, et, biometricmatch.Band{Review: 0.5, Confirm: 0.9}, "first", "test"); err != nil {
		t.Fatal(err)
	}
	set, err := biometricmatch.SetBand(ctx, sqlDB, et, biometricmatch.Band{Review: 0.6, Confirm: 0.8}, "second", "test")
	if err != nil {
		t.Fatal(err)
	}
	v, err = biometricmatch.CurrentBand(ctx, sqlDB, et, fallback)
	if err != nil || v.ID != set.ID || v.Band != set.Band || v.Source != "second" {
		t.Fatalf("CurrentBand = %+v, %v; want the latest version %+v", v, err, set)
	}
	if _, err := biometricmatch.SetBand(ctx, sqlDB, et, biometricmatch.Band{Review: 0.8, Confirm: 0.6}, "bad", ""); err == nil {
		t.Fatal("inverted band accepted")
	}

	// A probe on axis 0 and three faces at similarity 0.90, 0.65 and 0.30 to it, placed so that
	// they are far from each other (0.90 and 0.65 on opposite sides of axis 1, 0.30 on axis 2).
	in := cases.CaseInput{CaseID: "TEST-BAND", CaseType: cases.CaseTypeCriminal, Modality: "FACIAL",
		Evidences: []cases.EvidenceInput{{Sequence: 1}}}
	for s := int16(1); s <= 4; s++ {
		in.Evidences[0].Traces = append(in.Evidences[0].Traces, cases.TraceInput{Sequence: s, TraceType: "FACE_RECORD"})
	}
	res, err := cases.Ingest(ctx, sqlDB, in)
	if err != nil {
		t.Fatal(err)
	}
	s := func(c float64) float64 { return math.Sqrt(1 - c*c) }
	vecs := []string{unitVec(1), unitVec(0.9, s(0.9)), unitVec(0.65, -s(0.65)), unitVec(0.3, 0, s(0.3))}
	ids := make([]string, len(vecs))
	for i, tr := range res.Evidences[0].Traces {
		if _, err := db.New(sqlDB).UpsertFeatureEmbedding(ctx, db.UpsertFeatureEmbeddingParams{
			BiometricfeatureID: tr.FeatureID, EmbeddingType: et, Embedding: vecs[i],
		}); err != nil {
			t.Fatal(err)
		}
		ids[i] = graph.QuestionedFeatureID(tr.TraceID)
	}

	stats, err := biometricmatch.RunBand(ctx, sqlDB, et, v, "TEST")
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
	}{{ids[1], "POSITIVE", 0.9, 0.8}, {ids[2], "INCONCLUSIVE", 0.65, 0.6}}
	i := 0
	for rows.Next() {
		var a, b, decision, ref, kind string
		var conf, thr float64
		if err := rows.Scan(&a, &b, &decision, &conf, &thr, &ref, &kind); err != nil {
			t.Fatal(err)
		}
		w := want[i]
		pair := map[string]bool{a: true, b: true}
		if !pair[ids[0]] || !pair[w.other] || decision != w.decision || math.Abs(conf-w.conf) > 1e-4 || thr != w.thr ||
			ref != fmt.Sprint(set.ID) || kind != "match_threshold" {
			t.Fatalf("decision %d = %s|%s %s conf %.4f thr %v ref %s/%s; want probe|%s %s conf %v thr %v ref %d/match_threshold",
				i, a, b, decision, conf, thr, kind, ref, w.other, w.decision, w.conf, w.thr, set.ID)
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("%d decisions, want %d", i, len(want))
	}
}
