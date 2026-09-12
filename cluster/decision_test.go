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
