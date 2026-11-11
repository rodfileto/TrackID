package geocode

import (
	"math"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// simplify compares names regardless of case, accents and punctuation: "São João d'Aliança" is
// "SAO JOAO D ALIANCA".
func simplify(name string) string {
	stripped, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn))), name)
	if err != nil {
		stripped = name
	}
	return strings.Join(strings.FieldsFunc(strings.ToUpper(stripped), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

// flatten is text in upper case with its spacing collapsed, accents kept.
func flatten(text string) string {
	return strings.Join(strings.Fields(strings.ToUpper(text)), " ")
}

// similarity is 1 minus the edit distance over the longer length, on letters and digits only.
func similarity(a, b string) float64 {
	x, y := keepLettersAndDigits(a), keepLettersAndDigits(b)
	longest := max(len(x), len(y))
	if longest == 0 {
		return 1
	}
	return 1 - float64(editDistance(x, y))/float64(longest)
}

func keepLettersAndDigits(text string) []rune {
	var out []rune
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, r)
		}
	}
	return out
}

// editDistance counts the letters to insert, delete or change, and neighbours to swap, to turn a
// into b: a swap ("Saregnto") is one slip, not two.
func editDistance(a, b []rune) int {
	// Three rows: the one before last, for swaps.
	before, last, row := make([]int, len(b)+1), make([]int, len(b)+1), make([]int, len(b)+1)
	for j := range last {
		last[j] = j
	}
	for i := range a {
		row[0] = i + 1
		for j := range b {
			cost := 1
			if a[i] == b[j] {
				cost = 0
			}
			row[j+1] = min(last[j+1]+1, row[j]+1, last[j]+cost)
			if i > 0 && j > 0 && a[i] == b[j-1] && a[i-1] == b[j] {
				row[j+1] = min(row[j+1], before[j-1]+1)
			}
		}
		before, last, row = last, row, before
	}
	return last[len(b)]
}

// distanceMeters is the great-circle distance between two points.
func distanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000
	rad := math.Pi / 180
	a := math.Pow(math.Sin((lat2-lat1)*rad/2), 2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Pow(math.Sin((lon2-lon1)*rad/2), 2)
	return 2 * earthRadius * math.Asin(math.Sqrt(a))
}
