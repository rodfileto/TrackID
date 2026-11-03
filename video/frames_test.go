package video

import "testing"

func TestNearestTrackFaces(t *testing.T) {
	tracks := []Track{
		{Track: 0, Faces: []Face{{TimestampMs: 1000, Box: [4]float32{0, 0, 10, 10}}, {TimestampMs: 2000, Box: [4]float32{1, 0, 11, 10}}}},
		{Track: 1, Faces: []Face{{TimestampMs: 2000, Box: [4]float32{50, 0, 60, 10}}, {TimestampMs: 3000, Box: [4]float32{51, 0, 61, 10}}}},
		// Processed before every face was kept: only the best one, at 2 s.
		{Track: 2, TimestampMs: 2000, Box: [4]float32{90, 0, 99, 10}},
	}
	got := nearestTrackFaces(tracks, 2100)
	if len(got) != 3 {
		t.Fatalf("faces at 2.1 s = %+v, want the three at 2 s", got)
	}
	for _, tf := range got {
		if tf.face.TimestampMs != 2000 {
			t.Fatalf("not the nearest sample: %+v", tf)
		}
	}
	if got := nearestTrackFaces(tracks, 2900); len(got) != 1 || got[0].track != 1 {
		t.Fatalf("faces at 2.9 s = %+v", got)
	}
	if got := nearestTrackFaces(tracks, 10_000); len(got) != 0 {
		t.Fatalf("faces far from any sample = %+v", got)
	}
}
