package presence

import (
	"slices"
	"testing"
	"time"
)

// Belo Horizonte's centre, and offsets in metres from it: a degree of latitude is ~111 km, of longitude ~105 km here.
const bhLon, bhLat = -43.9378, -19.9208

func at(eastM, northM float64, id int64) Point {
	return Point{ID: id, Lon: bhLon + eastM/105000, Lat: bhLat + northM/111000}
}

func ids(a Area) []int64 {
	out := slices.Clone(a.PointIDs)
	slices.Sort(out)
	return out
}

func TestTwoSeparatedGroupsAndALonePoint(t *testing.T) {
	day := func(d int) *time.Time { t := time.Date(2026, 3, d, 0, 0, 0, 0, time.UTC); return &t }
	points := []Point{
		{ID: 1, Lon: at(0, 0, 0).Lon, Lat: at(0, 0, 0).Lat, At: day(1), Types: []string{"FRAUD"}, Reviewed: true},
		{ID: 2, Lon: at(200, 100, 0).Lon, Lat: at(200, 100, 0).Lat, At: day(5), Types: []string{"FRAUD"}},
		{ID: 3, Lon: at(-150, 120, 0).Lon, Lat: at(-150, 120, 0).Lat, At: day(9), Types: []string{"DAMAGE"}},
		{ID: 10, Lon: at(20000, 0, 0).Lon, Lat: at(20000, 0, 0).Lat, At: day(20)},     // 20 km east
		{ID: 11, Lon: at(20100, 200, 0).Lon, Lat: at(20100, 200, 0).Lat, At: day(25)}, // beside it
		{ID: 99, Lon: at(0, 60000, 0).Lon, Lat: at(0, 60000, 0).Lat},                  // 60 km north, alone
	}
	res := Areas(points, 500, 2)
	if len(res.Areas) != 2 || !slices.Equal(res.Noise, []int64{99}) || res.Mapped != 6 {
		t.Fatalf("areas %d, noise %v, mapped %d; want 2, [99], 6", len(res.Areas), res.Noise, res.Mapped)
	}
	main, other := res.Areas[0], res.Areas[1]
	if !slices.Equal(ids(main), []int64{1, 2, 3}) || !main.Main || main.Rank != 1 || !slices.Equal(ids(other), []int64{10, 11}) || other.Main {
		t.Errorf("main %v (main %v), other %v (main %v)", ids(main), main.Main, ids(other), other.Main)
	}
	if main.Reviewed != 1 || main.Types[0] != (TypeCount{"FRAUD", 2}) || !main.First.Equal(*day(1)) || !main.Last.Equal(*day(9)) {
		t.Errorf("main area: %+v", main)
	}
	if main.Share < 0.49 || main.Share > 0.51 {
		t.Errorf("share %.2f; want 3 of 6", main.Share)
	}
	// Its centre is among its points, and its radius holds most of them.
	if d := DistanceM(main.Centre[0], main.Centre[1], bhLon, bhLat); d > 300 {
		t.Errorf("centre %.0f m from the group", d)
	}
	if main.RadiusM < 100 || main.RadiusM > 400 {
		t.Errorf("radius %.0f m", main.RadiusM)
	}
	// The hull is a closed ring holding every point.
	if len(main.Hull) < 4 || main.Hull[0] != main.Hull[len(main.Hull)-1] {
		t.Errorf("hull not closed: %v", main.Hull)
	}
}

func TestEpsIsTheDistanceBetweenNeighbours(t *testing.T) {
	a, b := at(0, 0, 1), at(1000, 0, 2)
	d := DistanceM(a.Lon, a.Lat, b.Lon, b.Lat)
	if res := Areas([]Point{a, b}, d+1, 2); len(res.Areas) != 1 {
		t.Errorf("two points %.0f m apart, eps %.0f: %d areas, want 1", d, d+1, len(res.Areas))
	}
	if res := Areas([]Point{a, b}, d-1, 2); len(res.Areas) != 0 || len(res.Noise) != 2 {
		t.Errorf("two points %.0f m apart, eps %.0f: %d areas, noise %v, want none and both noise", d, d-1, len(res.Areas), res.Noise)
	}
}

func TestAChainIsOneArea(t *testing.T) {
	// Each point is 400 m from the next, so the ends are 1.6 km apart but reach each other through the middle.
	var points []Point
	for i := 0; i < 5; i++ {
		points = append(points, at(float64(i)*400, 0, int64(i+1)))
	}
	if res := Areas(points, 500, 2); len(res.Areas) != 1 || len(res.Areas[0].PointIDs) != 5 {
		t.Errorf("chain: %d areas", len(res.Areas))
	}
}

func TestMinPtsAndTooFewForAMainArea(t *testing.T) {
	points := []Point{at(0, 0, 1), at(100, 0, 2)}
	res := Areas(points, 500, 2)
	if len(res.Areas) != 1 || res.Areas[0].Main {
		t.Errorf("two points are an area but not the main one: %+v", res.Areas)
	}
	if res := Areas(points, 500, 3); len(res.Areas) != 0 || len(res.Noise) != 2 {
		t.Errorf("minPts 3 with two points: %+v", res)
	}
}

func TestPointsOnTheSameSpotKeepAVisibleArea(t *testing.T) {
	same := []Point{at(0, 0, 1), at(0, 0, 2), at(0, 0, 3)}
	res := Areas(same, 500, 2)
	if len(res.Areas) != 1 {
		t.Fatalf("areas %d", len(res.Areas))
	}
	a := res.Areas[0]
	if a.RadiusM < 100 || len(a.Hull) < 4 || a.Main != true {
		t.Errorf("coincident points: radius %.0f, hull %d, main %v", a.RadiusM, len(a.Hull), a.Main)
	}
}

func TestNothingToCluster(t *testing.T) {
	if res := Areas(nil, 500, 2); len(res.Areas) != 0 || len(res.Noise) != 0 || res.Mapped != 0 {
		t.Errorf("no points: %+v", res)
	}
	if res := Areas([]Point{at(0, 0, 1)}, 500, 2); len(res.Areas) != 0 || !slices.Equal(res.Noise, []int64{1}) {
		t.Errorf("one point: %+v", res)
	}
}

func TestTheMostRecentBreaksATie(t *testing.T) {
	old, recent := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	mk := func(east float64, id int64, when time.Time) Point { p := at(east, 0, id); p.At = &when; return p }
	points := []Point{mk(0, 1, old), mk(100, 2, old), mk(30000, 3, recent), mk(30100, 4, recent)}
	res := Areas(points, 500, 2)
	if len(res.Areas) != 2 || !slices.Equal(ids(res.Areas[0]), []int64{3, 4}) {
		t.Errorf("two areas of two: the recent one first, got %v", res.Areas)
	}
}
