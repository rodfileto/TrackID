package geocode

import (
	"regexp"
	"sort"
	"strings"
)

// ParsedAddress is a Brazilian address split for a structured search. Street keeps the number apart,
// since a street often has a match where its number has none. Place is the address before its
// municipality reduced with its locators (QUADRA 40 LOTE 14), when it has any: what places an
// address with no number. What ParseAddress can't tell stays empty.
type ParsedAddress struct {
	Street string
	Number string
	City   string
	UF     string
	Place  string
}

// states are the 27 federative units, by their name simplified (simplify).
var states = map[string]string{
	"ACRE": "AC", "ALAGOAS": "AL", "AMAPA": "AP", "AMAZONAS": "AM", "BAHIA": "BA", "CEARA": "CE",
	"DISTRITO FEDERAL": "DF", "ESPIRITO SANTO": "ES", "GOIAS": "GO", "MARANHAO": "MA", "MATO GROSSO": "MT",
	"MATO GROSSO DO SUL": "MS", "MINAS GERAIS": "MG", "PARA": "PA", "PARAIBA": "PB", "PARANA": "PR",
	"PERNAMBUCO": "PE", "PIAUI": "PI", "RIO DE JANEIRO": "RJ", "RIO GRANDE DO NORTE": "RN",
	"RIO GRANDE DO SUL": "RS", "RONDONIA": "RO", "RORAIMA": "RR", "SANTA CATARINA": "SC", "SAO PAULO": "SP",
	"SERGIPE": "SE", "TOCANTINS": "TO",
}

// ufCodes is the units' two-letter codes as a regular expression alternation.
var ufCodes = func() string {
	codes := make([]string, 0, len(states))
	for _, code := range states {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return strings.Join(codes, "|")
}()

// stateNames is the units' names as an alternation that takes them with or without accents, longest
// first so MATO GROSSO DO SUL isn't read as MATO GROSSO.
var stateNames = func() string {
	accents := strings.NewReplacer("A", "[AÁÂÃ]", "E", "[EÉÊ]", "I", "[IÍ]", "O", "[OÓÔÕ]", "U", "[UÚ]", "C", "[CÇ]")
	names := make([]string, 0, len(states))
	for name := range states {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for i, name := range names {
		names[i] = accents.Replace(name)
	}
	return strings.Join(names, "|")
}()

const (
	// A street's type, as addresses write it, including state and federal roads (BR-040, SP-330).
	streetTypes = `RUA|R\.|AVENIDA|AV\.?|PRA[CÇ]A|P[CÇ]A\.?|RODOVIA|ROD\.?|TRAVESSA|ALAMEDA|ESTRADA|VIELA|BECO|LADEIRA|LARGO|VICINAL|PASSAGEM|SERVID[ÃA]O|CAMINHO|BR[- ]?\d{3}`
)

var (
	cityAndUF = regexp.MustCompile(`([A-ZÀ-Ú][A-ZÀ-Ú'. ]{2,40}?)\s*(?:[/,-]|\s-\s)\s*(` + ufCodes + `|` + stateNames + `)\s*[.)]*\s*$`)
	// The street, then a comma and its number or "sem número": tried first, since a street's name
	// may end in a number (RUA 24, VICINAL VC 201) and the comma says where the name ends.
	streetCommaNumber = regexp.MustCompile(`^(?:.*?\b)?((?:` + streetTypes + `|(?:` + ufCodes + `)[- ]?\d{2,3})\s+[^,;:]{2,80}?)\s*,\s*(?:N[º°O.]*\s*)?(\d+[\w.-]*|S/?N[º°]?|SEM\s+N[ÚU]MERO|KM\s*\d+[\d,.]*)\b`)
	// The street, up to a number or "sem número", with or without a comma.
	streetAndNumber = regexp.MustCompile(`^(?:.*?\b)?((?:` + streetTypes + `|(?:` + ufCodes + `)[- ]?\d{2,3})\s+[^,;:]{2,80}?)[,\s]+(?:N[º°O.]*\s*)?(\d+[\w.-]*|S/?N[º°]?|SEM\s+N[ÚU]MERO|KM\s*\d+[\d,.]*)`)
	streetAlone     = regexp.MustCompile(`^(?:.*?\b)?((?:` + streetTypes + `|(?:` + ufCodes + `)[- ]?\d{2,3})\s+[^,;:]{2,80})`)
	// Words before a municipality's name. A bare "DE" isn't one: Juiz de Fora.
	cityLeadIn   = regexp.MustCompile(`^(?:.*\s)?(?:NA CIDADE DE|NO MUNIC[ÍI]PIO DE|MUNIC[ÍI]PIO DE|CIDADE DE|EM)\s+`)
	cepInAddress = regexp.MustCompile(`,?\s*CEP[:\s]*\d{2}\.?\d{3}-?\d{3}`)
	// The first locator with a value, where a street's name ends: RUA 24 QD 40 is RUA 24.
	firstLocator = regexp.MustCompile(`\b(?:QUADRA|QDA|QDR|QD|CONJUNTO|CONJ|CJ|LOTE|LT|CASA|CS|BLOCO|BL|CH[AÁ]CARA|CHAC|CH)\b\.?\s*(?:N[º°O.]?\s*)?(?:\d|[A-Z]\b)`)
)

// ParseAddress splits a Brazilian address as people write it: "Rua X, 120, Bairro, Cidade/UF",
// with the unit by its code or its name, a postcode or not, and a quadra, lote or casa in place of
// the number. A named place, such as an airport, has no street to find.
func ParseAddress(text string) ParsedAddress {
	flat := cepInAddress.ReplaceAllString(flatten(text), "")
	flat = strings.TrimRight(flat, " .;,")
	var parsed ParsedAddress
	if m := cityAndUF.FindStringSubmatch(flat); m != nil {
		parsed.City = strings.Trim(strings.TrimSpace(cityLeadIn.ReplaceAllString(m[1], "")), ".")
		parsed.UF = m[2]
		if len(parsed.UF) > 2 {
			parsed.UF = states[simplify(parsed.UF)]
		}
		flat = strings.TrimSpace(flat[:len(flat)-len(m[0])])
	}
	if located := parseLocators(flat); len(located.ends) > 0 {
		parsed.Place = located.String()
		if at := firstLocator.FindStringIndex(flat); at != nil {
			flat = strings.TrimRight(flat[:at[0]], " ,;-")
		}
	}
	m := streetCommaNumber.FindStringSubmatch(flat)
	if m == nil {
		m = streetAndNumber.FindStringSubmatch(flat)
	}
	if m != nil {
		parsed.Street = expandStreetType(strings.TrimSpace(m[1]))
		if number := m[2]; number != "" && number[0] >= '0' && number[0] <= '9' {
			parsed.Number = strings.TrimRight(number, ".-")
		}
	} else if m := streetAlone.FindStringSubmatch(flat); m != nil {
		parsed.Street = expandStreetType(strings.TrimSpace(m[1]))
	}
	return parsed
}

// expandStreetType writes out a street's abbreviated type. Only at its start: "R. " is also the end
// of "DR. " further along the name.
func expandStreetType(street string) string {
	for _, abbreviation := range []struct{ short, full string }{
		{"AV. ", "AVENIDA "}, {"AV ", "AVENIDA "}, {"R. ", "RUA "}, {"ROD. ", "RODOVIA "}, {"PÇA. ", "PRAÇA "}, {"PCA. ", "PRAÇA "}, {"PÇA ", "PRAÇA "}, {"PCA ", "PRAÇA "},
	} {
		if strings.HasPrefix(street, abbreviation.short) {
			return abbreviation.full + street[len(abbreviation.short):]
		}
	}
	return street
}

var streetWords = map[string]string{
	"AV": "AVENIDA", "R": "RUA", "ROD": "RODOVIA", "TV": "TRAVESSA", "TRAV": "TRAVESSA", "PCA": "PRACA", "AL": "ALAMEDA", "EST": "ESTRADA",
	"DR": "DOUTOR", "DRA": "DOUTORA", "PROF": "PROFESSOR", "PROFA": "PROFESSORA", "PRES": "PRESIDENTE", "CEL": "CORONEL",
	"GAL": "GENERAL", "GEN": "GENERAL", "MAL": "MARECHAL", "SEN": "SENADOR", "DEP": "DEPUTADO", "VER": "VEREADOR",
	"ENG": "ENGENHEIRO", "DES": "DESEMBARGADOR", "GOV": "GOVERNADOR", "PE": "PADRE", "STA": "SANTA", "STO": "SANTO", "S": "SAO",
	"CAP": "CAPITAO", "TEN": "TENENTE", "SGT": "SARGENTO", "BRIG": "BRIGADEIRO", "MIN": "MINISTRO",
}

var streetConnectors = map[string]bool{"DE": true, "DA": true, "DO": true, "DAS": true, "DOS": true, "E": true}

// StreetKey is a street's name reduced so spellings of it compare equal: no case, accents or
// connecting words, and abbreviations written out. "R. Dr. João da Silva" is "RUA DOUTOR JOAO SILVA".
func StreetKey(parts ...string) string {
	var words []string
	for _, word := range strings.Fields(simplify(strings.Join(parts, " "))) {
		if streetConnectors[word] {
			continue
		}
		if full, ok := streetWords[word]; ok {
			word = full
		}
		words = append(words, word)
	}
	return strings.Join(words, " ")
}
