package distribution

import (
	"reflect"
	"testing"

	"github.com/rodfileto/trackid/analyses"
)

func TestPlan(t *testing.T) {
	review, comparison := analyses.FacialReview, analyses.FacialComparison
	pending := []PendingWork{
		{ID: 1, CaseID: "C1", Type: review},
		{ID: 2, CaseID: "C2", Type: review},
		{ID: 3, CaseID: "C1", Sequence: 2, Type: comparison},
		{ID: 4, CaseID: "C3", Type: review},
		{ID: 5, CaseID: "C4", Type: review},
	}
	// C5's review awaits verification: bia examined it, so she is never its verifier.
	verification := []PendingWork{{ID: 6, CaseID: "C5", Type: review, Step: analyses.StepReviewer, Exclude: []int64{2}}}
	ana := Examiner{UserID: 1, Username: "ana", ExaminesFacial: true, OpenWork: 2}
	bia := Examiner{UserID: 2, Username: "bia", ExaminesFacial: true, ExaminesFingerprint: true}
	caio := Examiner{UserID: 3, Username: "caio", ExaminesFacial: true}
	fp := Examiner{UserID: 4, Username: "fp", ExaminesFingerprint: true}

	who := func(as []Assignment) []string {
		var out []string
		for _, a := range as {
			out = append(out, a.CaseID+":"+a.Username)
		}
		return out
	}

	tests := []struct {
		name       string
		examiners  []Examiner
		count      int
		want       []string
		noExaminer []int64
	}{
		// bia and caio start at 0, ana at 2: bia, caio, bia, caio, then all three at 2 -> ana.
		{"least loaded first, ties to the lowest id, load counted as it goes",
			[]Examiner{ana, bia, caio, fp}, 5,
			[]string{"C1:bia", "C2:caio", "C1:bia", "C3:caio", "C4:ana"}, nil},
		{"count stops it, the rest stay pending",
			[]Examiner{ana, bia, caio}, 2,
			[]string{"C1:bia", "C2:caio"}, nil},
		{"nobody examines faces: every analysis is skipped and stays pending",
			[]Examiner{fp}, 2,
			nil, []int64{1, 2, 3, 4, 5}},
		{"no examiners",
			nil, 3,
			nil, []int64{1, 2, 3, 4, 5}},
	}
	if got, _ := plan(verification, []Examiner{bia, caio}, 1); len(got) != 1 || got[0].Username != "caio" || got[0].Step != analyses.StepReviewer {
		t.Errorf("verification of bia's own analysis went to %+v, want caio as REVIEWER", got)
	}
	if got, none := plan(verification, []Examiner{bia}, 1); len(got) != 0 || len(none) != 1 {
		t.Errorf("only its first examiner available: assigned %+v, want it left pending", got)
	}
	// C6's comparison is on pairs bia and caio decided in its review: ana takes it, though she has
	// the most open work; with only bia and caio, the less loaded of the two does.
	compared := []PendingWork{{ID: 7, CaseID: "C6", Type: comparison, Avoid: []int64{2, 3}}}
	if got, _ := plan(compared, []Examiner{ana, bia, caio}, 1); len(got) != 1 || got[0].Username != "ana" {
		t.Errorf("comparison went to %+v, want ana, who never decided its pairs", got)
	}
	if got, _ := plan(compared, []Examiner{bia, {UserID: 3, Username: "caio", ExaminesFacial: true, OpenWork: 0}, {UserID: 5, Username: "eva", ExaminesFacial: true, OpenWork: 1}}, 1); len(got) != 1 || got[0].Username != "eva" {
		t.Errorf("comparison went to %+v, want eva, the one eligible examiner who never decided its pairs", got)
	}
	if got, _ := plan(compared, []Examiner{{UserID: 2, Username: "bia", ExaminesFacial: true, OpenWork: 3}, caio}, 1); len(got) != 1 || got[0].Username != "caio" {
		t.Errorf("only examiners who decided its pairs: went to %+v, want caio, the less loaded", got)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assigned, noExaminer := plan(pending, tt.examiners, tt.count)
			if got := who(assigned); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("assigned %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(noExaminer, tt.noExaminer) {
				t.Errorf("noExaminer %v, want %v", noExaminer, tt.noExaminer)
			}
		})
	}
}
