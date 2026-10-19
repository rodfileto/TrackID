package cases

import (
	"reflect"
	"testing"

	"github.com/rodfileto/trackid/cluster"
)

func TestShortestPaths(t *testing.T) {
	edges := map[string][]string{
		"A": {"B"},
		"B": {"A", "C", "D"},
		"C": {"B", "E"},
		"D": {"B", "E"},
		"E": {"C", "D"},
		"X": {"Y"}, // another component: never reached from A
		"Y": {"X"},
	}
	got := shortestPaths([]string{"A"}, edges)
	want := map[string][]string{
		"A": {"A"},
		"B": {"A", "B"},
		"C": {"A", "B", "C"},
		"D": {"A", "B", "D"},
		"E": {"A", "B", "C", "E"}, // C before D in B's (sorted) neighbours
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shortestPaths from A = %v, want %v", got, want)
	}

	// From several sources at once, each feature is reached from the nearest one.
	got = shortestPaths([]string{"A", "E"}, edges)
	if p := got["D"]; !reflect.DeepEqual(p, []string{"E", "D"}) {
		t.Errorf("D from {A, E} = %v, want [E D]", p)
	}
	if p := got["B"]; !reflect.DeepEqual(p, []string{"A", "B"}) {
		t.Errorf("B from {A, E} = %v, want [A B]", p)
	}
	if _, ok := got["X"]; ok {
		t.Errorf("X reached from {A, E}; it is not connected")
	}

	// A source with no edges reaches only itself.
	if got := shortestPaths([]string{"Z"}, edges); !reflect.DeepEqual(got, map[string][]string{"Z": {"Z"}}) {
		t.Errorf("shortestPaths from Z = %v", got)
	}
}

func TestIdentityStatus(t *testing.T) {
	sys := func(d cluster.Decision) cluster.ChainEntry { return cluster.ChainEntry{Role: cluster.RoleSystem, Decision: d} }
	in := func(role cluster.Role, d cluster.Decision) cluster.ChainEntry {
		return cluster.ChainEntry{Role: role, Decision: d, Analysis: 4}
	}
	P, N, I := cluster.DecisionPositive, cluster.DecisionNegative, cluster.DecisionInconclusive
	V, R, X := cluster.RoleVerificator, cluster.RoleReviewer, cluster.RoleInconsistence
	tests := []struct {
		name   string
		chain  []cluster.ChainEntry
		want   string
		wantOK bool
	}{
		{"a SYSTEM POSITIVE awaits review", []cluster.ChainEntry{sys(P)}, IdentityPending, true},
		{"a candidate in the review band awaits review", []cluster.ChainEntry{sys(I)}, IdentityPending, true},
		{"a first examiner alone does not settle it", []cluster.ChainEntry{sys(I), in(V, P)}, IdentityPending, true},
		{"a SYSTEM POSITIVE with a first examiner still awaits review", []cluster.ChainEntry{sys(P), in(V, N)}, IdentityPending, true},
		{"examiners agreeing positive confirm", []cluster.ChainEntry{sys(I), in(V, P), in(R, P)}, IdentityConfirmed, true},
		{"examiners agreeing negative exclude", []cluster.ChainEntry{sys(P), in(V, N), in(R, N)}, IdentityExcluded, true},
		{"examiners both inconclusive", []cluster.ChainEntry{sys(I), in(V, I), in(R, I)}, IdentityInconclusive, true},
		{"examiners disagreeing", []cluster.ChainEntry{sys(I), in(V, P), in(R, N)}, IdentityDisputed, true},
		{"the inconsistence step settling positive", []cluster.ChainEntry{sys(I), in(V, P), in(R, N), in(X, P)}, IdentityConfirmed, true},
		{"the inconsistence step settling inconclusive", []cluster.ChainEntry{sys(I), in(V, P), in(R, N), in(X, I)}, IdentityInconclusive, true},
		{"a comparison pair with no matcher score", []cluster.ChainEntry{in(V, P), in(R, P)}, IdentityConfirmed, true},
		{"below the review threshold (SYSTEM NEGATIVE)", []cluster.ChainEntry{sys(N)}, "", false},
		{"no decision", nil, "", false},
	}
	for _, tt := range tests {
		got, ok := identityStatus(tt.chain)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("%s: identityStatus = %q, %v; want %q, %v", tt.name, got, ok, tt.want, tt.wantOK)
		}
	}
}
