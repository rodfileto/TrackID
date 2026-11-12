// Package presence finds the main areas where points gather (docs/presence-areas-plan.md): the occurrences a person or
// a cluster appears in, grouped by density. It takes points and does not know where they come from.
//
// The method is DBSCAN: a point with at least MinPts points (itself included) within Eps metres is a core point; an
// area is the core points that reach one another within Eps, with the points within Eps of them. A point reaching no
// core point is noise: an isolated occurrence, not an area. Distances are great-circle metres, so no projection is
// needed.
package presence

import (
	"math"
	"sort"
	"time"
)

// Point is one occurrence with a place.
type Point struct {
	ID       int64
	Lon, Lat float64
	At       *time.Time // when it happened, nil if unknown
	Reviewed bool       // reached through a link examiners confirmed
	Types    []string   // occurrence type codes
}

// TypeCount is how many of an area's points have a type.
type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// Area is a group of points close to one another.
type Area struct {
	Rank     int          `json:"rank"` // 1 is the main area
	Main     bool         `json:"main"`
	PointIDs []int64      `json:"pointIds"`
	Centre   [2]float64   `json:"centre"`  // lon, lat: the mean of its points
	RadiusM  float64      `json:"radiusM"` // holding 80 % of its points
	Hull     [][2]float64 `json:"hull"`    // a closed ring, lon, lat, padded so a point on the edge is inside
	First    *time.Time   `json:"first,omitempty"`
	Last     *time.Time   `json:"last,omitempty"`
	Types    []TypeCount  `json:"types"`
	Reviewed int          `json:"reviewed"` // points reached through confirmed links
	Share    float64      `json:"share"`    // of all the points clustered
}

// Result is the areas found, the isolated points left out, and what was asked.
type Result struct {
	Areas  []Area  `json:"areas"`
	Noise  []int64 `json:"noise"`
	Mapped int     `json:"mapped"` // points clustered
	EpsM   float64 `json:"epsM"`
	MinPts int     `json:"minPts"`
}

// MinMainPoints is how many points it takes to call one area the main one: with fewer, the points are shown and no area
// is singled out.
const MinMainPoints = 3

// hullPadM pads a hull outward, so the points on its edge are inside it.
const hullPadM = 60

// minRadiusM keeps an area of coincident points visible.
const minRadiusM = 100

const earthM = 6371008.8

func rad(d float64) float64 { return d * math.Pi / 180 }

// DistanceM is the great-circle distance between two points, in metres.
func DistanceM(lon1, lat1, lon2, lat2 float64) float64 {
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthM * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Areas groups points by density. Areas are ranked by size, ties by the most recent; the first is Main when it has at
// least MinMainPoints points.
func Areas(points []Point, epsM float64, minPts int) Result {
	res := Result{Areas: []Area{}, Noise: []int64{}, Mapped: len(points), EpsM: epsM, MinPts: minPts}
	n := len(points)
	if n == 0 {
		return res
	}
	if minPts < 1 {
		minPts = 1
	}
	near := func(i int) []int {
		var out []int
		for j := range points {
			if DistanceM(points[i].Lon, points[i].Lat, points[j].Lon, points[j].Lat) <= epsM {
				out = append(out, j)
			}
		}
		return out
	}
	const unvisited, noise = -2, -1
	label := make([]int, n)
	for i := range label {
		label[i] = unvisited
	}
	clusters := 0
	for i := range points {
		if label[i] != unvisited {
			continue
		}
		neighbours := near(i)
		if len(neighbours) < minPts {
			label[i] = noise
			continue
		}
		label[i] = clusters
		queue := append([]int(nil), neighbours...)
		for len(queue) > 0 {
			j := queue[0]
			queue = queue[1:]
			if label[j] == noise {
				label[j] = clusters // a border point
			}
			if label[j] != unvisited {
				continue
			}
			label[j] = clusters
			if more := near(j); len(more) >= minPts {
				queue = append(queue, more...)
			}
		}
		clusters++
	}

	groups := make([][]Point, clusters)
	for i, l := range label {
		if l == noise {
			res.Noise = append(res.Noise, points[i].ID)
			continue
		}
		groups[l] = append(groups[l], points[i])
	}
	for _, g := range groups {
		res.Areas = append(res.Areas, describe(g, n))
	}
	sort.SliceStable(res.Areas, func(a, b int) bool {
		x, y := res.Areas[a], res.Areas[b]
		if len(x.PointIDs) != len(y.PointIDs) {
			return len(x.PointIDs) > len(y.PointIDs)
		}
		return later(x.Last, y.Last)
	})
	for i := range res.Areas {
		res.Areas[i].Rank = i + 1
	}
	if len(res.Areas) > 0 && len(res.Areas[0].PointIDs) >= MinMainPoints {
		res.Areas[0].Main = true
	}
	return res
}

// later reports whether a is after b; a time known beats one unknown.
func later(a, b *time.Time) bool {
	switch {
	case a == nil:
		return false
	case b == nil:
		return true
	}
	return a.After(*b)
}

func describe(points []Point, total int) Area {
	a := Area{PointIDs: make([]int64, len(points)), Types: []TypeCount{}, Share: float64(len(points)) / float64(total)}
	var sumLon, sumLat float64
	types := map[string]int{}
	for i, p := range points {
		a.PointIDs[i] = p.ID
		sumLon += p.Lon
		sumLat += p.Lat
		if p.Reviewed {
			a.Reviewed++
		}
		for _, t := range p.Types {
			types[t]++
		}
		if p.At != nil {
			if a.First == nil || p.At.Before(*a.First) {
				t := *p.At
				a.First = &t
			}
			if a.Last == nil || p.At.After(*a.Last) {
				t := *p.At
				a.Last = &t
			}
		}
	}
	a.Centre = [2]float64{sumLon / float64(len(points)), sumLat / float64(len(points))}
	for t, c := range types {
		a.Types = append(a.Types, TypeCount{t, c})
	}
	sort.Slice(a.Types, func(i, j int) bool {
		if a.Types[i].Count != a.Types[j].Count {
			return a.Types[i].Count > a.Types[j].Count
		}
		return a.Types[i].Type < a.Types[j].Type
	})
	if len(a.Types) > 3 {
		a.Types = a.Types[:3]
	}
	dist := make([]float64, len(points))
	for i, p := range points {
		dist[i] = DistanceM(a.Centre[0], a.Centre[1], p.Lon, p.Lat)
	}
	sort.Float64s(dist)
	a.RadiusM = math.Max(dist[int(math.Ceil(0.8*float64(len(dist))))-1], minRadiusM)
	a.Hull = hull(points, a.Centre, a.RadiusM)
	return a
}

// hull is the convex hull of the points, padded outward by hullPadM; for fewer than three points, or points on a line,
// a circle of the area's radius.
func hull(points []Point, centre [2]float64, radiusM float64) [][2]float64 {
	// Metres east and north of the centre, then back: the area is small enough for a flat local map.
	kx := earthM * math.Cos(rad(centre[1])) * math.Pi / 180
	ky := earthM * math.Pi / 180
	type xy struct{ x, y float64 }
	pts := make([]xy, len(points))
	for i, p := range points {
		pts[i] = xy{(p.Lon - centre[0]) * kx, (p.Lat - centre[1]) * ky}
	}
	sort.Slice(pts, func(i, j int) bool {
		if pts[i].x != pts[j].x {
			return pts[i].x < pts[j].x
		}
		return pts[i].y < pts[j].y
	})
	cross := func(o, a, b xy) float64 { return (a.x-o.x)*(b.y-o.y) - (a.y-o.y)*(b.x-o.x) }
	var lower, upper []xy
	for _, p := range pts {
		for len(lower) >= 2 && cross(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}
	for i := len(pts) - 1; i >= 0; i-- {
		p := pts[i]
		for len(upper) >= 2 && cross(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}
	ring := append(lower[:len(lower)-1], upper[:len(upper)-1]...)
	back := func(x, y float64) [2]float64 { return [2]float64{centre[0] + x/kx, centre[1] + y/ky} }
	var out [][2]float64
	if len(ring) < 3 {
		const sides = 48
		for i := 0; i < sides; i++ {
			angle := 2 * math.Pi * float64(i) / sides
			out = append(out, back(radiusM*math.Cos(angle), radiusM*math.Sin(angle)))
		}
	} else {
		for _, p := range ring {
			d := math.Hypot(p.x, p.y)
			if d == 0 {
				out = append(out, back(p.x, p.y))
				continue
			}
			out = append(out, back(p.x+p.x/d*hullPadM, p.y+p.y/d*hullPadM))
		}
	}
	return append(out, out[0])
}
