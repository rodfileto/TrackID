package geocode

import (
	"regexp"
	"strings"
)

// Addresses with no number, as the Distrito Federal and much of Goiás write them: a quadra,
// conjunto, lote or casa in place of the number. CNEFE keeps the street field and these locators
// apart; both sides are reduced here to the same words, so an address and CNEFE's compare as one key.

// locatorWords are the words that, followed by a value, say where on a street or sector a place is,
// written out.
var locatorWords = map[string]string{
	"QUADRA": "QUADRA", "QD": "QUADRA", "QDA": "QUADRA", "QDR": "QUADRA",
	"CONJUNTO": "CONJUNTO", "CONJ": "CONJUNTO", "CJ": "CONJUNTO", "CNJ": "CONJUNTO",
	"LOTE": "LOTE", "LT": "LOTE", "LOT": "LOTE",
	"CASA": "CASA", "CS": "CASA",
	"BLOCO": "BLOCO", "BL": "BLOCO", "BLC": "BLOCO",
	"CHACARA": "CHACARA", "CHAC": "CHACARA", "CH": "CHACARA",
	"TORRE": "TORRE", "EDIFICIO": "EDIFICIO", "ED": "EDIFICIO",
	"MODULO": "MODULO", "MOD": "MODULO", "PROJECAO": "PROJECAO", "PROJ": "PROJECAO",
	"GLEBA": "GLEBA",
}

// coarseLocators cover several buildings; a place found only down to one is about a block.
var coarseLocators = map[string]bool{"QUADRA": true, "CONJUNTO": true, "GLEBA": true}

// unitWords name a unit inside a place, which CNEFE gives as a separate address on the same point:
// true when a value follows.
var unitWords = map[string]bool{
	"APARTAMENTO": true, "APTO": true, "APT": true, "AP": true, "SALA": true, "SL": true, "LOJA": true, "LJ": true,
	"ANDAR": true, "BOX": true, "KITNET": true, "QUITINETE": true, "QUARTO": true, "COBERTURA": true, "SUBSOLO": true,
	"SOBRELOJA": true, "GARAGEM": true, "PORTA": true, "COMODO": true,
	"FRENTE": false, "FUNDOS": false, "FUNDO": false, "TERREO": false, "SOBRADO": false, "LADO": false, "ANEXO": false,
}

var (
	numberWords     = map[string]bool{"N": true, "NO": true, "NR": true, "NUM": true, "NUMERO": true}
	streetTypeWords = map[string]bool{"RUA": true, "AVENIDA": true, "ALAMEDA": true, "TRAVESSA": true, "ESTRADA": true,
		"RODOVIA": true, "PRACA": true, "VIELA": true, "BECO": true}
	gluedLocator = regexp.MustCompile(`^(QDA|QDR|QD|LT|CJ|BL|CS|CH)(\d+[A-Z]?)$`)
	ordinalMarks = strings.NewReplacer("º", " ", "°", " ", "ª", " ")
)

// locatorKey is an address reduced to words, with where its locators are.
type locatorKey struct {
	words  []string
	starts []int    // where each locator's word is
	ends   []int    // just after each locator's value
	kinds  []string // each locator's word, written out
}

func isLocatorValue(word string) bool {
	if len(word) == 1 {
		return true
	}
	return strings.ContainsAny(word, "0123456789")
}

func withoutLeadingZeros(word string) string {
	trimmed := strings.TrimLeft(word, "0")
	if trimmed == "" || trimmed[0] < '0' || trimmed[0] > '9' {
		return word
	}
	return trimmed
}

// parseLocators reduces text as StreetKey does, dropping units, with each locator's word written
// out and its value without leading zeros: "QR 308, CONJ. 13, LT. 08, APTO 101" is
// "QR 308 CONJUNTO 13 LOTE 8".
func parseLocators(text string) locatorKey {
	var raw []string
	for _, word := range strings.Fields(simplify(ordinalMarks.Replace(text))) {
		if m := gluedLocator.FindStringSubmatch(word); m != nil {
			raw = append(raw, m[1], m[2])
		} else {
			raw = append(raw, word)
		}
	}
	// value finds the value after position i, past a "Nº": the index of it, or -1.
	value := func(i int) int {
		j := i + 1
		for j < len(raw) && numberWords[raw[j]] && j+1 < len(raw) && isLocatorValue(raw[j+1]) {
			j++
		}
		if j < len(raw) && isLocatorValue(raw[j]) {
			return j
		}
		return -1
	}
	var key locatorKey
	for i := 0; i < len(raw); i++ {
		word := raw[i]
		if kind, ok := locatorWords[word]; ok {
			if j := value(i); j >= 0 {
				key.starts = append(key.starts, len(key.words))
				key.words = append(key.words, kind, withoutLeadingZeros(raw[j]))
				key.ends = append(key.ends, len(key.words))
				key.kinds = append(key.kinds, kind)
				i = j
				continue
			}
		}
		if takesValue, ok := unitWords[word]; ok {
			if takesValue {
				if j := value(i); j >= 0 {
					i = j
				}
			}
			continue
		}
		if streetConnectors[word] {
			continue
		}
		if full, ok := streetWords[word]; ok {
			word = full
		}
		key.words = append(key.words, withoutLeadingZeros(word)) // QNO 04 is QNO 4
	}
	return key
}

// String is the whole reduction, which parses back to the same key.
func (k locatorKey) String() string {
	return strings.Join(k.words, " ")
}

// placeCandidate is a key to look up, from the most exact.
type placeCandidate struct {
	key      string
	locators int // how many of the address's locators it keeps
	last     string
}

// candidates are the keys an address could be filed under in CNEFE: ending after each locator, from
// the last, and starting at each word before the first locator, since words before it may be
// anything. A street the address names is kept, so a lote on one street isn't found on another, and
// a sector it names is kept unless two locators remain: SQN 103 BLOCO B is not any BLOCO B.
func (k locatorKey) candidates() []placeCandidate {
	if len(k.ends) == 0 {
		return nil
	}
	lastStart := k.starts[0]
	for i := 0; i < k.starts[0]; i++ {
		if streetTypeWords[k.words[i]] {
			lastStart = i
			break
		}
	}
	var out []placeCandidate
	for n := len(k.ends); n >= 1; n-- {
		for start := 0; start <= lastStart; start++ {
			if start == k.starts[0] && start > 0 && n < 2 {
				continue
			}
			out = append(out, placeCandidate{strings.Join(k.words[start:k.ends[n-1]], " "), n, k.kinds[n-1]})
		}
	}
	return out
}

// filedKeys are the keys CNEFE files an address under: its whole chain of locators and each shorter
// one, so a lote not found still finds its quadra.
func (k locatorKey) filedKeys() []string {
	out := make([]string, 0, len(k.ends))
	for n := len(k.ends); n >= 1; n-- {
		out = append(out, strings.Join(k.words[:k.ends[n-1]], " "))
	}
	return out
}
