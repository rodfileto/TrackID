package cluster

import "testing"

func TestDeriveEdgeStatus(t *testing.T) {
	tests := []struct {
		name   string
		chain  []ChainEntry
		want   EdgeStatus
		wantOK bool
	}{
		{
			name:   "empty chain has no status",
			chain:  nil,
			wantOK: false,
		},
		{
			name:   "system alone positive confirms",
			chain:  []ChainEntry{{Role: RoleSystem, Decision: DecisionPositive}},
			want:   EdgeStatusConfirmed,
			wantOK: true,
		},
		{
			name:   "system alone inconclusive pending review",
			chain:  []ChainEntry{{Role: RoleSystem, Decision: DecisionInconclusive}},
			want:   EdgeStatusPendingReview,
			wantOK: true,
		},
		{
			name:   "system alone negative rejects",
			chain:  []ChainEntry{{Role: RoleSystem, Decision: DecisionNegative}},
			want:   EdgeStatusRejected,
			wantOK: true,
		},
		{
			name:   "verificator alone pending review regardless of decision",
			chain:  []ChainEntry{{Role: RoleVerificator, Decision: DecisionNegative}},
			want:   EdgeStatusPendingReview,
			wantOK: true,
		},
		{
			name: "verificator and reviewer agree positive confirms",
			chain: []ChainEntry{
				{Role: RoleVerificator, Decision: DecisionPositive},
				{Role: RoleReviewer, Decision: DecisionPositive},
			},
			want:   EdgeStatusConfirmed,
			wantOK: true,
		},
		{
			name: "verificator and reviewer agree negative rejects",
			chain: []ChainEntry{
				{Role: RoleVerificator, Decision: DecisionNegative},
				{Role: RoleReviewer, Decision: DecisionNegative},
			},
			want:   EdgeStatusRejected,
			wantOK: true,
		},
		{
			name: "verificator and reviewer both inconclusive stays pending review",
			chain: []ChainEntry{
				{Role: RoleVerificator, Decision: DecisionInconclusive},
				{Role: RoleReviewer, Decision: DecisionInconclusive},
			},
			want:   EdgeStatusPendingReview,
			wantOK: true,
		},
		{
			name: "verificator and reviewer disagree disputes",
			chain: []ChainEntry{
				{Role: RoleVerificator, Decision: DecisionPositive},
				{Role: RoleReviewer, Decision: DecisionNegative},
			},
			want:   EdgeStatusDisputed,
			wantOK: true,
		},
		{
			name: "inconsistence positive confirms despite prior disagreement",
			chain: []ChainEntry{
				{Role: RoleVerificator, Decision: DecisionPositive},
				{Role: RoleReviewer, Decision: DecisionNegative},
				{Role: RoleInconsistence, Decision: DecisionPositive},
			},
			want:   EdgeStatusConfirmed,
			wantOK: true,
		},
		{
			name: "inconsistence negative rejects despite prior disagreement",
			chain: []ChainEntry{
				{Role: RoleVerificator, Decision: DecisionPositive},
				{Role: RoleReviewer, Decision: DecisionNegative},
				{Role: RoleInconsistence, Decision: DecisionNegative},
			},
			want:   EdgeStatusRejected,
			wantOK: true,
		},
		{
			name: "inconsistence inconclusive leaves pair disputed",
			chain: []ChainEntry{
				{Role: RoleVerificator, Decision: DecisionPositive},
				{Role: RoleReviewer, Decision: DecisionNegative},
				{Role: RoleInconsistence, Decision: DecisionInconclusive},
			},
			want:   EdgeStatusDisputed,
			wantOK: true,
		},
		{
			name: "system row ignored once verificator and reviewer agree",
			chain: []ChainEntry{
				{Role: RoleSystem, Decision: DecisionPositive},
				{Role: RoleVerificator, Decision: DecisionNegative},
				{Role: RoleReviewer, Decision: DecisionNegative},
			},
			want:   EdgeStatusRejected,
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DeriveEdgeStatus(tt.chain)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("DeriveEdgeStatus(%+v) = %v, want %v", tt.chain, got, tt.want)
			}
		})
	}
}

// TestDeriveEdgeStatusRounds: a pair decided in several rounds keeps the outcome of its latest
// settled round (CONFIRMED or REJECTED); a later round that is pending, disputed or inconclusive
// does not undo it.
func TestDeriveEdgeStatusRounds(t *testing.T) {
	sysInconclusive := ChainEntry{Role: RoleSystem, Decision: DecisionInconclusive}
	sysPositive := ChainEntry{Role: RoleSystem, Decision: DecisionPositive}
	review := func(role Role, d Decision) ChainEntry { return ChainEntry{Role: role, Decision: d, Analysis: 7} }
	comparison := func(role Role, d Decision) ChainEntry { return ChainEntry{Role: role, Decision: d, Analysis: 9} }
	reviewConfirmed := []ChainEntry{sysInconclusive, review(RoleVerificator, DecisionPositive), review(RoleReviewer, DecisionPositive)}
	with := func(base []ChainEntry, more ...ChainEntry) []ChainEntry {
		return append(append([]ChainEntry{}, base...), more...)
	}

	tests := []struct {
		name      string
		chain     []ChainEntry
		want      EdgeStatus
		wantRound int64
	}{
		{"a review agreeing positive confirms", reviewConfirmed, EdgeStatusConfirmed, 7},
		{"a comparison under way does not unconfirm it",
			with(reviewConfirmed, comparison(RoleVerificator, DecisionPositive)), EdgeStatusConfirmed, 7},
		{"nor a comparison both examiners found inconclusive (no quality)",
			with(reviewConfirmed, comparison(RoleVerificator, DecisionInconclusive), comparison(RoleReviewer, DecisionInconclusive)), EdgeStatusConfirmed, 7},
		{"nor a disputed comparison",
			with(reviewConfirmed, comparison(RoleVerificator, DecisionPositive), comparison(RoleReviewer, DecisionNegative)), EdgeStatusConfirmed, 7},
		{"a comparison settling negative rejects what the review confirmed",
			with(reviewConfirmed, comparison(RoleVerificator, DecisionNegative), comparison(RoleReviewer, DecisionNegative)), EdgeStatusRejected, 9},
		{"a comparison settling positive after a rejecting review confirms",
			[]ChainEntry{sysInconclusive, review(RoleVerificator, DecisionNegative), review(RoleReviewer, DecisionNegative),
				comparison(RoleVerificator, DecisionPositive), comparison(RoleReviewer, DecisionPositive)}, EdgeStatusConfirmed, 9},
		{"an inconsistence settles its own round",
			[]ChainEntry{sysInconclusive, review(RoleVerificator, DecisionPositive), review(RoleReviewer, DecisionNegative),
				review(RoleInconsistence, DecisionPositive)}, EdgeStatusConfirmed, 7},
		{"a SYSTEM POSITIVE stays confirmed while a review is under way",
			[]ChainEntry{sysPositive, review(RoleVerificator, DecisionNegative)}, EdgeStatusConfirmed, 0},
		{"until the review settles it",
			[]ChainEntry{sysPositive, review(RoleVerificator, DecisionNegative), review(RoleReviewer, DecisionNegative)}, EdgeStatusRejected, 7},
		{"with nothing settled, the latest round's state stands",
			[]ChainEntry{sysInconclusive, review(RoleVerificator, DecisionPositive), review(RoleReviewer, DecisionNegative),
				comparison(RoleVerificator, DecisionPositive)}, EdgeStatusPendingReview, 9},
		{"rows in any order",
			[]ChainEntry{comparison(RoleReviewer, DecisionNegative), review(RoleReviewer, DecisionPositive), sysInconclusive,
				comparison(RoleVerificator, DecisionNegative), review(RoleVerificator, DecisionPositive)}, EdgeStatusRejected, 9},
		{"human rows from before analyses are round 0, with the SYSTEM row",
			[]ChainEntry{sysPositive, {Role: RoleVerificator, Decision: DecisionNegative}, review(RoleVerificator, DecisionPositive)}, EdgeStatusPendingReview, 7},
	}
	for _, tt := range tests {
		round, got, ok := DecidingRound(tt.chain)
		if !ok || got != tt.want || round != tt.wantRound {
			t.Errorf("%s: got %s in round %d (%v); want %s in round %d", tt.name, got, round, ok, tt.want, tt.wantRound)
		}
	}
}
