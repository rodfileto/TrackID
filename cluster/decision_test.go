package cluster

import "testing"

func TestDeriveEdgeStatus(t *testing.T) {
	tests := []struct {
		name   string
		chain  []chainEntry
		want   string
		wantOK bool
	}{
		{
			name:   "empty chain has no status",
			chain:  nil,
			wantOK: false,
		},
		{
			name:   "system alone positive confirms",
			chain:  []chainEntry{{role: roleSystem, decision: decisionPositive}},
			want:   edgeStatusConfirmed,
			wantOK: true,
		},
		{
			name:   "system alone inconclusive pending review",
			chain:  []chainEntry{{role: roleSystem, decision: decisionInconclusive}},
			want:   edgeStatusPendingReview,
			wantOK: true,
		},
		{
			name:   "system alone negative rejects",
			chain:  []chainEntry{{role: roleSystem, decision: decisionNegative}},
			want:   edgeStatusRejected,
			wantOK: true,
		},
		{
			name:   "verificator alone pending review regardless of decision",
			chain:  []chainEntry{{role: roleVerificator, decision: decisionNegative}},
			want:   edgeStatusPendingReview,
			wantOK: true,
		},
		{
			name: "verificator and reviewer agree positive confirms",
			chain: []chainEntry{
				{role: roleVerificator, decision: decisionPositive},
				{role: roleReviewer, decision: decisionPositive},
			},
			want:   edgeStatusConfirmed,
			wantOK: true,
		},
		{
			name: "verificator and reviewer agree negative rejects",
			chain: []chainEntry{
				{role: roleVerificator, decision: decisionNegative},
				{role: roleReviewer, decision: decisionNegative},
			},
			want:   edgeStatusRejected,
			wantOK: true,
		},
		{
			name: "verificator and reviewer both inconclusive stays pending review",
			chain: []chainEntry{
				{role: roleVerificator, decision: decisionInconclusive},
				{role: roleReviewer, decision: decisionInconclusive},
			},
			want:   edgeStatusPendingReview,
			wantOK: true,
		},
		{
			name: "verificator and reviewer disagree disputes",
			chain: []chainEntry{
				{role: roleVerificator, decision: decisionPositive},
				{role: roleReviewer, decision: decisionNegative},
			},
			want:   edgeStatusDisputed,
			wantOK: true,
		},
		{
			name: "inconsistence positive confirms despite prior disagreement",
			chain: []chainEntry{
				{role: roleVerificator, decision: decisionPositive},
				{role: roleReviewer, decision: decisionNegative},
				{role: roleInconsistence, decision: decisionPositive},
			},
			want:   edgeStatusConfirmed,
			wantOK: true,
		},
		{
			name: "inconsistence negative rejects despite prior disagreement",
			chain: []chainEntry{
				{role: roleVerificator, decision: decisionPositive},
				{role: roleReviewer, decision: decisionNegative},
				{role: roleInconsistence, decision: decisionNegative},
			},
			want:   edgeStatusRejected,
			wantOK: true,
		},
		{
			name: "inconsistence inconclusive leaves pair disputed",
			chain: []chainEntry{
				{role: roleVerificator, decision: decisionPositive},
				{role: roleReviewer, decision: decisionNegative},
				{role: roleInconsistence, decision: decisionInconclusive},
			},
			want:   edgeStatusDisputed,
			wantOK: true,
		},
		{
			name: "system row ignored once verificator and reviewer agree",
			chain: []chainEntry{
				{role: roleSystem, decision: decisionPositive},
				{role: roleVerificator, decision: decisionNegative},
				{role: roleReviewer, decision: decisionNegative},
			},
			want:   edgeStatusRejected,
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
