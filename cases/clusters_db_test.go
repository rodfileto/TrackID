package cases_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/identity"
	"github.com/rodfileto/trackid/person"
)

// TestCaseClustersShowOnlyReviewedLinks walks decisions through cluster.Persist to the case page
// and the person profile: a SYSTEM match alone builds the AUTOMATIC layer only and never reaches
// either view; links people confirmed build the REVIEWED layer, shown direct or through a path of
// reviewed links; and revoking a reviewed link splits the reviewed cluster on the next run.
func TestCaseClustersShowOnlyReviewedLinks(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	q := db.New(sqlDB)

	face := func(caseID string) string {
		t.Helper()
		res, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: caseID, CaseType: cases.CaseTypeCriminal, Modality: "FACIAL",
			Description: "case " + caseID, Evidences: []cases.EvidenceInput{{Sequence: 1,
				File: &cases.FileInput{StorageRef: "bucket/" + caseID, ContentType: "image/jpeg", HashID: "hash-" + caseID},
				Traces: []cases.TraceInput{{Sequence: 1, TraceType: "FACE_CAPTURE", Box: &cases.Box{X1: 1, Y1: 2, X2: 30, Y2: 40},
					Codifications: []cases.CodificationInput{{Sequence: 1, CodificationType: "FACE_EMBEDDING"}}}}}}})
		if err != nil {
			t.Fatal(err)
		}
		return graph.QuestionedFeatureID(res.Evidences[0].Traces[0].TraceID)
	}
	t1, t2, t3, t4 := face("C1"), face("C2"), face("C3"), face("C4")
	enrolled, err := identity.Ingest(ctx, sqlDB, identity.Enrollment{PersonID: "P1", DocumentType: "ID", DocumentNumber: "DOC-1",
		RegisterNumber: "REG-1", Name: "Pessoa Um", Parent1Name: "A", Parent1Gender: "U", Parent2Name: "B", Parent2Gender: "U",
		Files: []identity.FileInput{{FileType: "photo", StorageRef: "bucket/p1", ContentType: "image/jpeg"}}})
	if err != nil {
		t.Fatal(err)
	}
	k := graph.KnownFeatureID(enrolled.IdentityFileIDs["photo"])

	decideIn := func(analysisID int64, a, b, role, decision, by string) {
		t.Helper()
		if a > b {
			a, b = b, a
		}
		p := db.InsertBiometricDecisionParams{FeatureAID: a, FeatureBID: b, Modality: "FACIAL", Role: role, Decision: decision,
			CaseAnalysisID: sql.NullInt64{Int64: analysisID, Valid: analysisID != 0}}
		if role == "SYSTEM" {
			p.SystemSource = sql.NullString{String: "test", Valid: true}
			p.Confidence = sql.NullFloat64{Float64: 0.9, Valid: true}
		} else {
			p.Username = sql.NullString{String: by, Valid: true}
		}
		if _, err := q.InsertBiometricDecision(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	// decide records a decision outside any analysis: human rows like these count in round 0,
	// with the SYSTEM decision (see cluster.DecidingRound).
	decide := func(a, b, role, decision, by string) { t.Helper(); decideIn(0, a, b, role, decision, by) }
	reviewed := func(a, b string) {
		t.Helper()
		decide(a, b, "VERIFICATOR", "POSITIVE", "ana")
		decide(a, b, "REVIEWER", "POSITIVE", "bia")
	}
	decide(t1, t2, "SYSTEM", "POSITIVE", "")
	reviewed(t1, t2) // C1 - C2: reviewed, direct from C1
	decide(t2, t3, "SYSTEM", "INCONCLUSIVE", "")
	reviewed(t2, t3)                            // C2 - C3: reviewed, so C3 is reached from C1 through C2
	reviewed(t3, k)                             // C3 - P1: reviewed, so P1 is reached from C1 through C2 and C3
	decide(t1, k, "SYSTEM", "INCONCLUSIVE", "") // C1 - P1: a candidate in the review band, not reviewed
	c1Analyses, err := analyses.List(ctx, sqlDB, "C1")
	if err != nil || len(c1Analyses) == 0 {
		t.Fatalf("C1's analyses = %v, %v; want its automatic FACIAL_REVIEW", c1Analyses, err)
	}
	decide(t1, t4, "SYSTEM", "POSITIVE", "")
	decideIn(c1Analyses[0].ID, t1, t4, "VERIFICATOR", "POSITIVE", "ana") // C1 - C4: a SYSTEM match, its review still pending

	st, err := cluster.Run(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	// AUTOMATIC: {t1..t4, k}. REVIEWED: {t1, t2, t3, k} and t4 alone.
	if st.Clusters != 1 || st.Evidence != 5 || st.Reviewed.Clusters != 2 || st.Reviewed.Evidence != 5 {
		t.Fatalf("stats = %+v; want 1 automatic cluster and 2 reviewed ones over 5 features", st)
	}

	describe := func(c cases.CaseCluster) string {
		var parts []string
		for _, l := range c.Links {
			name := l.CaseID
			if l.Kind == "KNOWN" {
				name = l.PersonID
			}
			var via []string
			for _, s := range l.Path {
				via = append(via, s.CaseID+s.PersonID)
			}
			if l.Direct {
				parts = append(parts, fmt.Sprintf("%s direct by %s (%s)", name, l.DecidedBy, l.DecisionRole))
			} else {
				parts = append(parts, fmt.Sprintf("%s via %s", name, strings.Join(via, ">")))
			}
		}
		return strings.Join(parts, "; ")
	}
	caseClusters := func(caseID string) []cases.CaseCluster {
		t.Helper()
		got, err := cases.ListCaseClusters(ctx, sqlDB, caseID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	c1 := caseClusters("C1")
	if len(c1) != 1 {
		t.Fatalf("C1 clusters = %+v; want one", c1)
	}
	if got, want := describe(c1[0]), "C2 direct by bia (REVIEWER); C3 via C2; P1 via C2>C3"; got != want {
		t.Fatalf("C1 links = %q; want %q (and never C4, an automatic match)", got, want)
	}
	reviewedID := c1[0].ClusterID
	if l := c1[0].Links[2]; l.FeatureID != k || l.FromTraceID == 0 || graph.QuestionedFeatureID(l.FromTraceID) != t1 {
		t.Fatalf("P1's link = %+v; want its feature id and C1's face as the start of the path", l)
	}
	if c4 := caseClusters("C4"); len(c4) != 0 {
		t.Fatalf("C4 clusters = %+v; want none: no reviewed decision links its face to anything", c4)
	}

	// The case's decisions on identities list every candidate at the review threshold, settled
	// or not: C1's face has P1 awaiting review (still only indirect in the clusters), C3's has P1
	// confirmed.
	identities := func(caseID string) string {
		t.Helper()
		faces, err := cases.ListCaseIdentityDecisions(ctx, sqlDB, caseID)
		if err != nil {
			t.Fatal(err)
		}
		var parts []string
		for _, f := range faces {
			for _, d := range f.Identities {
				parts = append(parts, fmt.Sprintf("%s %s %s", d.PersonID, d.Status, d.DecidedBy))
			}
		}
		return fmt.Sprint(len(faces), parts)
	}
	if got, want := identities("C1"), "1 [P1 PENDING ]"; got != want {
		t.Fatalf("C1 identity decisions = %s; want %s", got, want)
	}
	if got, want := identities("C3"), "1 [P1 CONFIRMED bia]"; got != want {
		t.Fatalf("C3 identity decisions = %s; want %s", got, want)
	}
	if got, want := identities("C4"), "1 []"; got != want {
		t.Fatalf("C4 identity decisions = %s; want its face with no identity", got)
	}

	profile, err := person.GetProfile(ctx, sqlDB, "P1")
	if err != nil {
		t.Fatal(err)
	}
	var related []string
	for _, c := range profile.Cases {
		related = append(related, c.CaseID)
	}
	if fmt.Sprint(related) != "[C1 C2 C3]" || len(profile.Clusters) != 1 || profile.Clusters[0].ClusterID != reviewedID {
		t.Fatalf("P1's cases = %v, clusters = %+v; want C1-C3 through the reviewed cluster, never C4", related, profile.Clusters)
	}

	overview, err := cluster.ListOverview(ctx, sqlDB, cluster.ListOverviewParams{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if overview.Total != 1 || overview.Items[0].MemberCount != 5 || overview.Items[0].ClusterID == reviewedID {
		t.Fatalf("overview = %+v; want only the automatic cluster, C4 included", overview.Items)
	}

	// Revoking C2 - C3 splits the reviewed cluster. The part holding its anchor -- the smallest
	// feature id, P1's photo -- keeps the cluster number, so C3 and P1 keep it and C1 and C2 move
	// to a new cluster.
	decide(t2, t3, "INCONSISTENCE", "NEGATIVE", "caio")
	if _, err := cluster.Run(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	c1 = caseClusters("C1")
	if len(c1) != 1 || c1[0].ClusterID == reviewedID || describe(c1[0]) != "C2 direct by bia (REVIEWER)" {
		t.Fatalf("C1 after revoking C2 - C3 = %+v; want a new reviewed cluster with C2 only", c1)
	}
	if c3 := caseClusters("C3"); len(c3) != 1 || c3[0].ClusterID != reviewedID || describe(c3[0]) != "P1 direct by bia (REVIEWER)" {
		t.Fatalf("C3 after revoking = %+v; want cluster %d with P1", c3, reviewedID)
	}
}
