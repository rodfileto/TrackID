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
		ConclusionIdentification: DecisionPositive, ConclusionLimitedSupport: DecisionPositive,
		ConclusionInconclusive: DecisionInconclusive, ConclusionLimitedExclusion: DecisionNegative, ConclusionExclusion: DecisionNegative,
	} {
		if got := c.Decision(); got != want {
			t.Errorf("%s maps to %s, want %s", c, got, want)
		}
	}
	if !ConclusionIdentification.AtLeast(ConclusionStrongSupport) || !ConclusionStrongSupport.AtLeast(ConclusionStrongSupport) ||
		ConclusionModerateSupport.AtLeast(ConclusionStrongSupport) || ConclusionInconclusive.AtLeast(ConclusionLimitedSupport) {
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
		{"a strong laudo replaces the lead", join([]ChainEntry{sys}, review(DecisionPositive), laudo(ConclusionStrongSupport, ConclusionStrongSupport)),
			Link{Basis: LinkLaudo, Conclusion: ConclusionStrongSupport, Identified: true}, true},
		{"a laudo below the level supports without identifying", join(review(DecisionPositive), laudo(ConclusionLimitedSupport, ConclusionLimitedSupport)),
			Link{Basis: LinkLaudo, Conclusion: ConclusionLimitedSupport}, true},
		{"an inconclusive laudo leaves the lead standing", join(review(DecisionPositive), laudo(ConclusionInconclusive, ConclusionInconclusive)),
			Link{Basis: LinkLead, Identified: true, LaterLaudo: "INCONCLUSIVE"}, true},
		{"examiners disagreeing on the level dispute the laudo", join(review(DecisionPositive), laudo(ConclusionStrongSupport, ConclusionModerateSupport)),
			Link{Basis: LinkLead, Identified: true, LaterLaudo: "DISPUTED"}, true},
		{"a laudo still pending", join(review(DecisionPositive), laudo(ConclusionStrongSupport, ConclusionStrongSupport)[:1]),
			Link{Basis: LinkLead, Identified: true, LaterLaudo: "PENDING"}, true},
		{"an excluding laudo rejects the pair", join(review(DecisionPositive), laudo(ConclusionExclusion, ConclusionExclusion)), Link{}, false},
		{"a third examiner's level settles a disputed laudo", join(review(DecisionPositive), laudo(ConclusionStrongSupport, ConclusionModerateSupport),
			[]ChainEntry{{Role: RoleInconsistence, Decision: DecisionPositive, Conclusion: ConclusionIdentification, Analysis: 2, AnalysisType: comparisonType}}),
			Link{Basis: LinkLaudo, Conclusion: ConclusionIdentification, Identified: true}, true},
	}
	for _, tt := range tests {
		got, ok := ReviewedLink(tt.chain, ConclusionStrongSupport)
		if ok != tt.ok || got != tt.want {
			t.Errorf("%s: got %+v, %v; want %+v, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}
