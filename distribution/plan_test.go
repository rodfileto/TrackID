package distribution

import (
	"reflect"
	"testing"

	"github.com/rodfileto/trackid/analyses"
)

func TestPlan(t *testing.T) {
	review, comparison := analyses.FacialReview, analyses.FacialComparison
	pending := []PendingAnalysis{
		{ID: 1, CaseID: "C1", Type: review},
		{ID: 2, CaseID: "C2", Type: review},
		{ID: 3, CaseID: "C1", Sequence: 2, Type: comparison},
		{ID: 4, CaseID: "C3", Type: review},
		{ID: 5, CaseID: "C4", Type: review},
	}
	ana := Examiner{UserID: 1, Username: "ana", ExaminesFacial: true, OpenAnalyses: 2}
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
