package cluster

import (
	"encoding/json"
	"strings"
	"testing"
)

// The overview keeps the graph's shape (kinds, sizes, edges) and nothing that names or identifies a
// person, a case or a feature.
func TestAnonymous(t *testing.T) {
	g := GraphResult{
		Nodes: []GraphNode{
			{ID: "person:SIM-P-000001", Kind: "person", Label: "Ana Maria Silva", PersonID: "SIM-P-000001"},
			{ID: "cluster:7", Kind: "cluster", Label: "Cluster 7", Modality: "FACIAL", Size: 12},
			{ID: "TRACE:42#feature", Kind: "trace", Label: "Trace 42", CaseID: "SIM-2022-00020"},
		},
		Edges: []GraphEdge{{Source: "person:SIM-P-000001", Target: "cluster:7"}, {Source: "cluster:7", Target: "TRACE:42#feature"},
			{Source: "cluster:7", Target: "missing"}},
	}
	a := Anonymous(g)
	if len(a.Nodes) != 3 || len(a.Edges) != 2 || a.Nodes[1].Kind != "cluster" || a.Nodes[1].Size != 12 {
		t.Fatalf("anonymous graph %+v", a)
	}
	if a.Edges[0] != (GraphEdge{Source: "n1", Target: "n2"}) || a.Edges[1] != (GraphEdge{Source: "n2", Target: "n3"}) {
		t.Errorf("edges %+v", a.Edges)
	}
	out, _ := json.Marshal(a)
	for _, leak := range []string{"Ana", "SIM-", "TRACE", "Cluster 7", "FACIAL", "42"} {
		if strings.Contains(string(out), leak) {
			t.Errorf("the anonymous graph carries %q: %s", leak, out)
		}
	}
}
