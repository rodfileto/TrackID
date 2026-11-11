package reports

import (
	"testing"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/examination"
)

// A fingerprint decision carries no level on the facial scale: a positive one is an
// identification, in a comparison and in a scene examination alike. A facial positive below the
// identification level stays support.
func TestResultByType(t *testing.T) {
	for _, c := range []struct {
		t        analyses.Type
		decision string
		level    cluster.Conclusion
		want     string
	}{
		{analyses.FingerprintComparison, examination.Positive, "", ResultIdentified},
		{analyses.CrimeSceneExamination, examination.Positive, "", ResultIdentified},
		{analyses.CrimeSceneExamination, examination.Negative, "", ResultExcluded},
		{analyses.FacialComparison, examination.Positive, "", ResultSupported},
		{analyses.FacialComparison, examination.Positive, cluster.DefaultIdentificationLevel, ResultIdentified},
		{analyses.FingerprintComparison, "", "", ResultUnsettled},
	} {
		if got := result(c.t, c.decision, c.level, cluster.DefaultIdentificationLevel); got != c.want {
			t.Errorf("%s %s %s: %s, want %s", c.t, c.decision, c.level, got, c.want)
		}
	}
}
