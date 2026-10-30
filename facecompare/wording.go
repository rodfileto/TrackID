package facecompare

import (
	"fmt"
	"strings"
	"unicode"
)

// Terms FISWG rules out, in Portuguese and English. For a feature, words that imply the two images
// share a source (Best Practices for Markups & Annotations v1.0, §5.2.1); in an opinion, the
// language of certainty (ACE-V one-to-one guideline v1.0, §5.3.2.2; OSAC 2022-S-0001, §4.4.2).
// They are matched as whole words, so "mesmo" also matches "mesmo assim": the check warns, the
// examiner decides.
var featureTerms = []string{
	"único", "única", "únicos", "únicas", "significativo", "significativa", "significativos", "significativas",
	"mesmo", "mesma", "mesmos", "mesmas", "idêntico", "idêntica", "idênticos", "idênticas",
	"exato", "exata", "exatos", "exatas",
	"unique", "significant", "same", "identical", "exact",
}

var opinionTerms = []string{
	"individualiza", "individualizar", "individualização", "individualizado", "individualizada",
	"identifica", "identificar", "identificação", "identificado", "identificada",
	"inclui", "inclusão", "exclui", "excluir", "exclusão", "excluído", "excluída",
	"individualize", "individualizes", "individualized", "individualization",
	"identify", "identifies", "identified", "identification",
	"include", "includes", "inclusion", "exclude", "excludes", "excluded", "exclusion",
}

// Text is a piece of the examiner's writing to check, and where it is.
type Text struct {
	Where string `json:"where"`
	Text  string `json:"text"`
}

// WordingWarnings returns a warning for each ruled-out term in texts. Both lists apply to every
// text: a finding's note can slip into an opinion's language, and the reasoning into a feature's.
func WordingWarnings(texts []Text) []Warning {
	var out []Warning
	for _, t := range texts {
		words := tokens(t.Text)
		for _, w := range words {
			switch {
			case containsFold(featureTerms, w):
				out = append(out, Warning{"WORDING_FEATURE", fmt.Sprintf("“%s”: o FISWG desaconselha termos que sugerem origem comum ao descrever uma característica (único, significativo, mesmo, idêntico, exato).", w), t.Where})
			case containsFold(opinionTerms, w):
				out = append(out, Warning{"WORDING_OPINION", fmt.Sprintf("“%s”: a opinião não se expressa como certeza; o FISWG e a OSAC excluem termos como identificação, individualização, inclusão e exclusão.", w), t.Where})
			}
		}
	}
	return out
}

// SessionTexts collects the examiner's writing in s for WordingWarnings.
func SessionTexts(s Session) []Text {
	var out []Text
	add := func(where, text string) {
		if strings.TrimSpace(text) != "" {
			out = append(out, Text{where, text})
		}
	}
	for _, side := range []struct {
		name string
		a    Analysis
	}{{"Q", s.Questioned.Analysis}, {"K", s.Known.Analysis}} {
		add(side.name+".notes", side.a.Notes)
		add(side.name+".suitabilityReason", side.a.SuitabilityReason)
		for _, f := range side.a.Factors {
			add(side.name+"."+f.Code, f.Notes)
		}
	}
	for _, f := range s.Findings {
		where := f.Characteristic
		if f.Side != "" {
			where += "/" + f.Side
		}
		add(where, f.Questioned)
		add(where, f.Known)
		add(where, f.Notes)
	}
	if s.Opinion != nil {
		add("opinion", s.Opinion.Reasoning)
	}
	return out
}

func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) })
}

func containsFold(list []string, w string) bool {
	for _, x := range list {
		if x == w {
			return true
		}
	}
	return false
}
