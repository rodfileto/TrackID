package ask

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var (
	numberPattern = regexp.MustCompile(`\d+(?:[.,]\d+)*`)
	// A list's numbering is not a claim: "1. Roubo".
	listMarker = regexp.MustCompile(`(?m)^\s*\d+[.)]\s`)
)

// unverifiedNumbers lists the numbers in an answer that appear in none of the sources (the tools'
// results and arguments, and the question). It does not say they are wrong -- a sum or a percentage
// the model worked out is listed too -- only that no query returned them, so a reader should check.
// A number that does appear can still be attached to the wrong thing: this catches invented figures,
// not misreadings.
func unverifiedNumbers(answer string, sources []string) []string {
	known := map[string]bool{}
	var values []float64
	for _, source := range sources {
		for _, token := range numberPattern.FindAllString(source, -1) {
			for _, reading := range numberReadings(token) {
				known[reading] = true
				if f, err := strconv.ParseFloat(reading, 64); err == nil {
					values = append(values, f)
				}
			}
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, token := range numberPattern.FindAllString(listMarker.ReplaceAllString(answer, ""), -1) {
		if seen[token] {
			continue
		}
		seen[token] = true
		found := false
		for _, reading := range numberReadings(token) {
			found = found || known[reading] || roundedFrom(reading, values)
		}
		if !found {
			out = append(out, token)
		}
	}
	return out
}

// numberReadings is what a written number can mean, in canonical form. Portuguese and English swap
// the separators, so "1.234" may be 1234 or 1.234 and "12,5" may be 12.5 or 125.
func numberReadings(token string) []string {
	readings := []string{canonicalNumber(strings.NewReplacer(".", "", ",", "").Replace(token))}
	if i := strings.LastIndexAny(token, ".,"); i >= 0 {
		whole := strings.NewReplacer(".", "", ",", "").Replace(token[:i])
		readings = append(readings, canonicalNumber(whole+"."+token[i+1:]))
	}
	return readings
}

// roundedFrom says whether a written decimal is some known value rounded to the same places: 688,97
// for a score of 688.975. A whole number gets no such allowance, since 13 for 12.5 is too easy to pass
// off, and a count has no decimals to round.
func roundedFrom(reading string, values []float64) bool {
	i := strings.IndexByte(reading, '.')
	if i < 0 {
		return false
	}
	places := len(reading) - i - 1
	for _, v := range values {
		if strconv.FormatFloat(v, 'f', places, 64) == strconv.FormatFloat(mustFloat(reading), 'f', places, 64) {
			return true
		}
	}
	return false
}

func mustFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func canonicalNumber(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// verificationSources gathers what an answer's numbers may come from.
func verificationSources(question string, focus []Entity, steps []Step) []string {
	sources := []string{question}
	for _, e := range focus {
		sources = append(sources, e.Ref, e.Label)
	}
	for _, s := range steps {
		sources = append(sources, string(s.Arguments), string(s.Result))
	}
	return sources
}

// jsonOrString keeps text that should be JSON as JSON, for a JSONB column: the model's tool
// arguments may not parse, and they are recorded all the same.
func jsonOrString(text string) json.RawMessage {
	if json.Valid([]byte(text)) {
		return json.RawMessage(text)
	}
	quoted, _ := json.Marshal(text)
	return quoted
}
