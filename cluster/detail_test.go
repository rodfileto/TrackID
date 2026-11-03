package cluster

import "testing"

func edgePair(a, b string) DetailPair {
	return DetailPair{FeatureA: a, FeatureB: b, Inside: true, Edge: true}
}

// Two triangles joined by one edge: that edge is the only bridge, its ends the only cut vertices.
func TestStructureBridges(t *testing.T) {
	members := []string{"a", "b", "c", "d", "e", "f"}
	pairs := []DetailPair{
		edgePair("a", "b"), edgePair("b", "c"), edgePair("a", "c"),
		edgePair("c", "d"),
		edgePair("d", "e"), edgePair("e", "f"), edgePair("d", "f"),
		{FeatureA: "a", FeatureB: "x", Inside: false}, // a boundary pair: not an edge
	}
	got, components := structure(members, pairs)
	if components != 1 {
		t.Fatalf("components = %d, want 1", components)
	}
	for i, p := range pairs {
		want := p.FeatureA == "c" && p.FeatureB == "d"
		if p.Bridge != want {
			t.Errorf("pair %d (%s-%s) bridge = %v, want %v", i, p.FeatureA, p.FeatureB, p.Bridge, want)
		}
	}
	wantDegree := map[string]int{"a": 2, "b": 2, "c": 3, "d": 3, "e": 2, "f": 2}
	for _, m := range got {
		if m.Degree != wantDegree[m.FeatureID] {
			t.Errorf("%s degree = %d, want %d", m.FeatureID, m.Degree, wantDegree[m.FeatureID])
		}
		if want := m.FeatureID == "c" || m.FeatureID == "d"; m.CutVertex != want {
			t.Errorf("%s cut vertex = %v, want %v", m.FeatureID, m.CutVertex, want)
		}
	}
}

// Members no edge joins (a stale cluster) count as separate components.
func TestStructureComponents(t *testing.T) {
	_, components := structure([]string{"a", "b", "c"}, []DetailPair{edgePair("a", "b")})
	if components != 2 {
		t.Fatalf("components = %d, want 2", components)
	}
	_, components = structure([]string{"a"}, nil)
	if components != 1 {
		t.Fatalf("singleton components = %d, want 1", components)
	}
}
