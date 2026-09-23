package cluster

import (
	"reflect"
	"testing"
)

const fp = "FINGERPRINT"

func TestReconcile(t *testing.T) {
	tests := []struct {
		name       string
		modalities map[string]string
		edges      []edge
		existing   map[int64]clusterState
		want       plan
	}{
		{
			name:       "empty",
			modalities: map[string]string{},
			edges:      nil,
			existing:   map[int64]clusterState{},
			want:       plan{kept: map[int64][]string{}},
		},
		{
			name:       "new cluster from an edge",
			modalities: map[string]string{"A": fp, "B": fp},
			edges:      []edge{{left: "A", right: "B", modality: fp}},
			existing:   map[int64]clusterState{},
			want: plan{
				kept:        map[int64][]string{},
				newClusters: []newCluster{{modality: fp, evidence: []string{"A", "B"}}},
			},
		},
		{
			name:       "singleton evidence",
			modalities: map[string]string{"A": fp},
			edges:      nil,
			existing:   map[int64]clusterState{},
			want: plan{
				kept:        map[int64][]string{},
				newClusters: []newCluster{{modality: fp, evidence: []string{"A"}}},
			},
		},
		{
			name:       "extend existing cluster",
			modalities: map[string]string{"A": fp, "B": fp, "C": fp},
			edges:      []edge{{left: "A", right: "B", modality: fp}, {left: "B", right: "C", modality: fp}},
			existing:   map[int64]clusterState{1: {modality: fp, members: []string{"A", "B"}}},
			want: plan{
				kept: map[int64][]string{1: {"A", "B", "C"}},
			},
		},
		{
			name:       "merge keeps oldest id",
			modalities: map[string]string{"A": fp, "B": fp, "C": fp, "D": fp},
			edges:      []edge{{left: "A", right: "B", modality: fp}, {left: "C", right: "D", modality: fp}, {left: "B", right: "C", modality: fp}},
			existing: map[int64]clusterState{
				3: {modality: fp, members: []string{"C", "D"}},
				5: {modality: fp, members: []string{"A", "B"}},
			},
			want: plan{
				kept:   map[int64][]string{3: {"A", "B", "C", "D"}},
				merges: []merge{{from: 5, to: 3}},
			},
		},
		{
			name:       "split keeps anchor, orphan forms new cluster",
			modalities: map[string]string{"A": fp, "B": fp, "C": fp},
			edges:      []edge{{left: "A", right: "B", modality: fp}},
			existing:   map[int64]clusterState{1: {modality: fp, members: []string{"A", "B", "C"}}},
			want: plan{
				kept:        map[int64][]string{1: {"A", "B"}},
				newClusters: []newCluster{{modality: fp, evidence: []string{"C"}}},
			},
		},
		{
			name:       "split orphan joins new evidence",
			modalities: map[string]string{"A": fp, "B": fp, "C": fp, "D": fp},
			edges:      []edge{{left: "A", right: "B", modality: fp}, {left: "C", right: "D", modality: fp}},
			existing:   map[int64]clusterState{1: {modality: fp, members: []string{"A", "B", "C"}}},
			want: plan{
				kept:        map[int64][]string{1: {"A", "B"}},
				newClusters: []newCluster{{modality: fp, evidence: []string{"C", "D"}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reconcile(tt.modalities, tt.edges, tt.existing)
			if !reflect.DeepEqual(got.kept, tt.want.kept) {
				t.Fatalf("kept = %v, want %v", got.kept, tt.want.kept)
			}
			if !reflect.DeepEqual(got.newClusters, tt.want.newClusters) {
				t.Fatalf("newClusters = %v, want %v", got.newClusters, tt.want.newClusters)
			}
			if !reflect.DeepEqual(got.merges, tt.want.merges) {
				t.Fatalf("merges = %v, want %v", got.merges, tt.want.merges)
			}
		})
	}
}
