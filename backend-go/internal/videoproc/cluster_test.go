package videoproc

import (
	"math"
	"testing"
)

// unit is a small helper to build a near-unit vector pointed mostly along
// one axis, with a bit of spread on the others -- cheap stand-in for "faces
// of the same person, slightly different angle/lighting."
func unit(primary int, dims int, jitter ...float32) []float32 {
	v := make([]float32, dims)
	v[primary] = 1
	for i, j := range jitter {
		if i < dims {
			v[i] += j
		}
	}

	var sumSq float64
	for _, x := range v {
		sumSq += float64(x) * float64(x)
	}
	norm := float32(math.Sqrt(sumSq))
	for i := range v {
		v[i] /= norm
	}
	return v
}

func TestDBSCANGroupsTightClustersAndIsolatesNoise(t *testing.T) {
	dims := 8
	var embeddings [][]float32
	// cluster A: 4 near-identical vectors around axis 0
	for i := 0; i < 4; i++ {
		embeddings = append(embeddings, unit(0, dims, 0, 0.01*float32(i)))
	}
	// cluster B: 3 near-identical vectors around axis 3
	for i := 0; i < 3; i++ {
		embeddings = append(embeddings, unit(3, dims, 0, 0, 0, 0, 0.01*float32(i)))
	}
	// two outliers, far from both clusters and each other
	embeddings = append(embeddings, unit(6, dims))
	embeddings = append(embeddings, unit(7, dims))

	labels := dbscan(embeddings, 0.05, 2)

	if labels[0] == -1 {
		t.Fatalf("expected point 0 to join a cluster, got noise")
	}
	for i := 1; i < 4; i++ {
		if labels[i] != labels[0] {
			t.Errorf("expected point %d to share cluster A's label %d, got %d", i, labels[0], labels[i])
		}
	}

	if labels[4] == -1 {
		t.Fatalf("expected point 4 to join a cluster, got noise")
	}
	for i := 5; i < 7; i++ {
		if labels[i] != labels[4] {
			t.Errorf("expected point %d to share cluster B's label %d, got %d", i, labels[4], labels[i])
		}
	}

	if labels[0] == labels[4] {
		t.Errorf("expected cluster A and cluster B to have different labels, both got %d", labels[0])
	}

	if labels[7] != -1 {
		t.Errorf("expected outlier 7 to be noise (-1), got %d", labels[7])
	}
	if labels[8] != -1 {
		t.Errorf("expected outlier 8 to be noise (-1), got %d", labels[8])
	}
}

func TestDBSCANMinSamplesBoundary(t *testing.T) {
	dims := 4
	// exactly minSamples=3 tight points should form a cluster
	embeddings := [][]float32{
		unit(0, dims, 0.001),
		unit(0, dims, -0.001),
		unit(0, dims, 0),
	}
	labels := dbscan(embeddings, 0.01, 3)
	for i, l := range labels {
		if l == -1 {
			t.Errorf("point %d: expected to join a cluster with exactly minSamples neighbors, got noise", i)
		}
	}

	// one point fewer than minSamples should all be noise
	embeddings = embeddings[:2]
	labels = dbscan(embeddings, 0.01, 3)
	for i, l := range labels {
		if l != -1 {
			t.Errorf("point %d: expected noise with fewer than minSamples neighbors, got cluster %d", i, l)
		}
	}
}
