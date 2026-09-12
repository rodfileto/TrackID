package cluster

import (
	"reflect"
	"testing"
)

func TestConnectedComponents(t *testing.T) {
	tests := []struct {
		name  string
		edges [][2]string
		want  [][]string
	}{
		{
			name:  "empty",
			edges: nil,
			want:  nil,
		},
		{
			name:  "single pair",
			edges: [][2]string{{"a", "b"}},
			want:  [][]string{{"a", "b"}},
		},
		{
			name:  "chain",
			edges: [][2]string{{"a", "b"}, {"b", "c"}},
			want:  [][]string{{"a", "b", "c"}},
		},
		{
			name:  "disjoint components",
			edges: [][2]string{{"a", "b"}, {"c", "d"}},
			want:  [][]string{{"a", "b"}, {"c", "d"}},
		},
		{
			name:  "singleton dropped",
			edges: [][2]string{{"a", "b"}, {"c", "d"}, {"e", "e"}},
			want:  [][]string{{"a", "b"}, {"c", "d"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := connectedComponents(tt.edges)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("connectedComponents(%v) = %v, want %v", tt.edges, got, tt.want)
			}
		})
	}
}
