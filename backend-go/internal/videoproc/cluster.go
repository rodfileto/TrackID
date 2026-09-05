package videoproc

// dbscan clusters embeddings by cosine distance. Returns one label per input
// point: cluster ids are 0-based and consecutive, -1 means noise.
//
// Standard DBSCAN: a point is a "core point" if at least minSamples points
// (itself included) lie within eps of it. Clusters grow outward from core
// points, absorbing every point within reach; a point that's merely near a
// cluster but isn't itself a core point (a "border point") joins the
// cluster without extending it further. Points nobody reaches stay noise.
func dbscan(embeddings [][]float32, eps float64, minSamples int) []int {
	n := len(embeddings)
	labels := make([]int, n)
	for i := range labels {
		labels[i] = -1
	}
	visited := make([]bool, n)

	neighbors := func(i int) []int {
		var ns []int
		for j := 0; j < n; j++ {
			if j != i && cosineDistance(embeddings[i], embeddings[j]) <= eps {
				ns = append(ns, j)
			}
		}
		return ns
	}

	nextLabel := 0
	for i := 0; i < n; i++ {
		if visited[i] {
			continue
		}
		visited[i] = true

		ns := neighbors(i)
		if len(ns)+1 < minSamples { // +1 counts the point itself
			continue // noise for now -- may still be absorbed later as a border point
		}

		label := nextLabel
		nextLabel++
		labels[i] = label

		queue := append([]int{}, ns...)
		for len(queue) > 0 {
			j := queue[0]
			queue = queue[1:]

			if !visited[j] {
				visited[j] = true
				if jNs := neighbors(j); len(jNs)+1 >= minSamples {
					queue = append(queue, jNs...)
				}
			}
			if labels[j] == -1 {
				labels[j] = label
			}
		}
	}
	return labels
}

// cosineDistance assumes both vectors are already L2-normalized (true of
// every embedding vision.Normalize produces), so the dot product is exactly
// the cosine similarity.
func cosineDistance(a, b []float32) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return 1 - dot
}
