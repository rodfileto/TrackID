package cluster

import "testing"

func TestConclusionScale(t *testing.T) {
	for _, c := range Conclusions {
		if !c.Valid() {
			t.Errorf("%s is not valid", c)
		}
	}
	if Conclusion("PROBABLE").Valid() {
		t.Error("an unknown level is valid")
	}
	for c, want := range map[Conclusion]Decision{
		ConclusionStrongSupportCommon: DecisionPositive, ConclusionSupportCommon: DecisionPositive,
		ConclusionInconclusive: DecisionInconclusive, ConclusionSupportDifferent: DecisionNegative, ConclusionStrongSupportDifferent: DecisionNegative,
	} {
		if got := c.Decision(); got != want {
			t.Errorf("%s maps to %s, want %s", c, got, want)
		}
	}
	if !ConclusionStrongSupportCommon.AtLeast(ConclusionStrongSupportCommon) || !ConclusionStrongSupportCommon.AtLeast(ConclusionSupportCommon) ||
		ConclusionSupportCommon.AtLeast(ConclusionStrongSupportCommon) || ConclusionInconclusive.AtLeast(ConclusionSupportCommon) ||
		ConclusionStrongSupportDifferent.AtLeast(ConclusionSupportCommon) {
		t.Error("AtLeast does not follow the scale")
	}
}

// TestReviewedLink covers the plan's decision 7: a lead identifies while it is the pair's only
// human round, a concluding laudo replaces it at its level, and one that ends inconclusive leaves
// the lead standing, shown beside it.
func TestReviewedLink(t *testing.T) {
	sys := ChainEntry{Role: RoleSystem, Decision: DecisionPositive}
	review := func(d Decision) []ChainEntry {
		return []ChainEntry{
			{Role: RoleVerificator, Decision: d, Analysis: 1, AnalysisType: "FACIAL_REVIEW"},
			{Role: RoleReviewer, Decision: d, Analysis: 1, AnalysisType: "FACIAL_REVIEW"},
		}
	}
	laudo := func(v, r Conclusion) []ChainEntry {
		return []ChainEntry{
			{Role: RoleVerificator, Decision: v.Decision(), Conclusion: v, Analysis: 2, AnalysisType: comparisonType},
			{Role: RoleReviewer, Decision: r.Decision(), Conclusion: r, Analysis: 2, AnalysisType: comparisonType},
		}
	}
	confronto := func(v, r Decision) []ChainEntry {
		return []ChainEntry{
			{Role: RoleVerificator, Decision: v, Analysis: 3, AnalysisType: fingerprintComparisonType},
			{Role: RoleReviewer, Decision: r, Analysis: 3, AnalysisType: fingerprintComparisonType},
		}
	}
	join := func(parts ...[]ChainEntry) []ChainEntry {
		var out []ChainEntry
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	tests := []struct {
		name  string
		chain []ChainEntry
		want  Link
		ok    bool
	}{
		{"system alone is not reviewed", []ChainEntry{sys}, Link{}, false},
		{"a lead alone identifies", join([]ChainEntry{sys}, review(DecisionPositive)), Link{Basis: LinkLead, Identified: true}, true},
		{"a strong laudo replaces the lead", join([]ChainEntry{sys}, review(DecisionPositive), laudo(ConclusionStrongSupportCommon, ConclusionStrongSupportCommon)),
			Link{Basis: LinkLaudo, Conclusion: ConclusionStrongSupportCommon, Identified: true}, true},
		{"a laudo below the level supports without identifying", join(review(DecisionPositive), laudo(ConclusionSupportCommon, ConclusionSupportCommon)),
			Link{Basis: LinkLaudo, Conclusion: ConclusionSupportCommon}, true},
		{"an inconclusive laudo leaves the lead standing", join(review(DecisionPositive), laudo(ConclusionInconclusive, ConclusionInconclusive)),
			Link{Basis: LinkLead, Identified: true, LaterLaudo: "INCONCLUSIVE"}, true},
		{"examiners disagreeing on the level dispute the laudo", join(review(DecisionPositive), laudo(ConclusionStrongSupportCommon, ConclusionSupportCommon)),
			Link{Basis: LinkLead, Identified: true, LaterLaudo: "DISPUTED"}, true},
		{"a laudo still pending", join(review(DecisionPositive), laudo(ConclusionStrongSupportCommon, ConclusionStrongSupportCommon)[:1]),
			Link{Basis: LinkLead, Identified: true, LaterLaudo: "PENDING"}, true},
		{"an excluding laudo rejects the pair", join(review(DecisionPositive), laudo(ConclusionStrongSupportDifferent, ConclusionStrongSupportDifferent)), Link{}, false},
		{"a third examiner's level settles a disputed laudo", join(review(DecisionPositive), laudo(ConclusionStrongSupportCommon, ConclusionSupportCommon),
			[]ChainEntry{{Role: RoleInconsistence, Decision: DecisionPositive, Conclusion: ConclusionStrongSupportCommon, Analysis: 2, AnalysisType: comparisonType}}),
			Link{Basis: LinkLaudo, Conclusion: ConclusionStrongSupportCommon, Identified: true}, true},
		{"a positive fingerprint comparison is a laudo that identifies", join([]ChainEntry{sys}, confronto(DecisionPositive, DecisionPositive)),
			Link{Basis: LinkLaudo, Identified: true}, true},
		{"a negative fingerprint comparison rejects the pair", join([]ChainEntry{sys}, confronto(DecisionNegative, DecisionNegative)), Link{}, false},
		{"a fingerprint comparison still pending is no link", join([]ChainEntry{sys}, confronto(DecisionPositive, DecisionPositive)[:1]), Link{}, false},
	}
	for _, tt := range tests {
		got, ok := ReviewedLink(tt.chain, ConclusionStrongSupportCommon)
		if ok != tt.ok || got != tt.want {
			t.Errorf("%s: got %+v, %v; want %+v, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}
