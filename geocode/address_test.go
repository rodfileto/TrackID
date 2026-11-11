package geocode

import (
	"strings"
	"testing"
)

func TestParseAddress(t *testing.T) {
	for text, want := range map[string]ParsedAddress{
		"Rua Goiás, 120, Centro, Belo Horizonte/MG":           {Street: "RUA GOIÁS", Number: "120", City: "BELO HORIZONTE", UF: "MG"},
		"Av. Paulista, nº 1000 - Bela Vista, São Paulo - SP":  {Street: "AVENIDA PAULISTA", Number: "1000", City: "SÃO PAULO", UF: "SP"},
		"R. Dr. Flores, 45, Porto Alegre, Rio Grande do Sul":  {Street: "RUA DR. FLORES", Number: "45", City: "PORTO ALEGRE", UF: "RS"},
		"Rua das Flores, 12, Campo Grande/MATO GROSSO DO SUL": {Street: "RUA DAS FLORES", Number: "12", City: "CAMPO GRANDE", UF: "MS"},
		"Rua do Sol 300, Cuiabá, Mato Grosso":                 {Street: "RUA DO SOL", Number: "300", City: "CUIABÁ", UF: "MT"},
		"Rodovia SP-330, km 98, Jundiaí/SP":                   {Street: "RODOVIA SP-330", Number: "", City: "JUNDIAÍ", UF: "SP"},
		"Rua Sem Número, s/n, CEP 70000-000, Brasília/DF":     {Street: "RUA SEM NÚMERO", City: "BRASÍLIA", UF: "DF"},
		"Aeroporto Internacional, Confins/MG":                 {City: "CONFINS", UF: "MG"},
		"Rua Goiás 120":                                       {Street: "RUA GOIÁS", Number: "120"},
		"Rua A, 730 - Senador Camará, Rio de Janeiro/RJ":      {Street: "RUA A", Number: "730", City: "RIO DE JANEIRO", UF: "RJ"},
		"R. L 40, Rio de Janeiro - RJ":                        {Street: "RUA L", Number: "40", City: "RIO DE JANEIRO", UF: "RJ"},
		"Tv. União 31, Rio de Janeiro - RJ":                   {Street: "TRAVESSA UNIÃO", Number: "31", City: "RIO DE JANEIRO", UF: "RJ"},
		"Estr. do Guandu 8301, Rio de Janeiro - RJ":           {Street: "ESTRADA DO GUANDU", Number: "8301", City: "RIO DE JANEIRO", UF: "RJ"},
		"Via B Oito, 6 - Maré, Rio de Janeiro/RJ":             {Street: "VIA B OITO", Number: "6", City: "RIO DE JANEIRO", UF: "RJ"},
		"Escadaria Selarón, 12, Rio de Janeiro/RJ":            {Street: "ESCADARIA SELARÓN", Number: "12", City: "RIO DE JANEIRO", UF: "RJ"},
		"QR 308, CONJ. 13, LT. 08 - Samambaia, Brasília/DF":   {City: "BRASÍLIA", UF: "DF", Place: "QR 308 CONJUNTO 13 LOTE 8 SAMAMBAIA"},
		"SQN 103 BLOCO B APTO 401, ASA NORTE, BRASILIA-DF":    {City: "BRASILIA", UF: "DF", Place: "SQN 103 BLOCO B ASA NORTE"},
		"Avenida Brasil, 100, casa 2, Betim/MG":               {Street: "AVENIDA BRASIL", Number: "100", City: "BETIM", UF: "MG", Place: "AVENIDA BRASIL 100 CASA 2"},
		"Rua 24 QD 40 LT 14, Jardim Brasília, Águas Lindas de Goiás/GO": {Street: "RUA 24", City: "ÁGUAS LINDAS DE GOIÁS", UF: "GO",
			Place: "RUA 24 QUADRA 40 LOTE 14 JARDIM BRASILIA"},
	} {
		if got := ParseAddress(text); got != want {
			t.Errorf("%q:\n got %+v\nwant %+v", text, got, want)
		}
	}
}

func TestEveryStateByName(t *testing.T) {
	if len(states) != 27 {
		t.Fatalf("%d federative units, want 27", len(states))
	}
	for name, uf := range states {
		if got := ParseAddress("Rua A, 1, Cidade Qualquer, " + name).UF; got != uf {
			t.Errorf("%s: got %q, want %s", name, got, uf)
		}
	}
	if got := ParseAddress("Rua A, 1, Vitória, Espírito Santo").UF; got != "ES" {
		t.Errorf("a name with accents: got %q", got)
	}
}

func TestStreetKey(t *testing.T) {
	for in, want := range map[string]string{
		"R. Dr. João da Silva":     "RUA DOUTOR JOAO SILVA",
		"RUA DOUTOR JOÃO DA SILVA": "RUA DOUTOR JOAO SILVA",
		"Av. Pres. Vargas":         "AVENIDA PRESIDENTE VARGAS",
	} {
		if got := StreetKey(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	if s := similarity("RUA GOIAS", "RUA GOIAS"); s != 1 {
		t.Errorf("the same: %v", s)
	}
	if s := similarity("RUA GOIAZ", "RUA GOIAS"); s < 0.85 || s >= 1 {
		t.Errorf("one letter apart: %v", s)
	}
	if s, one := similarity("RUA SAREGNTO REGO", "RUA SARGENTO REGO"), similarity("RUA SARGENTO REG", "RUA SARGENTO REGO"); s != one {
		t.Errorf("two neighbours swapped is one slip: %v, one letter missing %v", s, one)
	}
	if s := similarity("RUA A", "AVENIDA BRASIL"); s > 0.5 {
		t.Errorf("unrelated: %v", s)
	}
}

func TestParseLocators(t *testing.T) {
	for text, want := range map[string]string{
		"QR 308, CONJ. 13, LT. 08, APTO 101":          "QR 308 CONJUNTO 13 LOTE 8",
		"QR 308 CONJUNTO 13 LOTE 8":                   "QR 308 CONJUNTO 13 LOTE 8", // reduced again, the same
		"SQN 103 Bloco B, apartamento 401, Asa Norte": "SQN 103 BLOCO B ASA NORTE",
		"QUADRA 3 CONJUNTO E LOTE 1":                  "QUADRA 3 CONJUNTO E LOTE 1", // E is a value, not "and"
		"Av. Uruguaiana, Qd 22, Lt 4, Casa 1, fundos": "AVENIDA URUGUAIANA QUADRA 22 LOTE 4 CASA 1",
		"RUA 24 QD40 LT14":                            "RUA 24 QUADRA 40 LOTE 14",
		"QNO 04 CONJUNTO E LOTE 022":                  "QNO 4 CONJUNTO E LOTE 22",
		"CHÁCARA Nº 21, Núcleo Rural Cariru":          "CHACARA 21 NUCLEO RURAL CARIRU",
		"Rua da Casa Branca":                          "RUA CASA BRANCA",
	} {
		if got := parseLocators(text).String(); got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
	if n := len(parseLocators("Rua da Casa Branca").ends); n != 0 {
		t.Errorf("a casa with no value is a word: %d locators", n)
	}
}

func TestPlaceCandidates(t *testing.T) {
	keys := func(text string) []string {
		var out []string
		for _, c := range parseLocators(text).candidates() {
			out = append(out, c.key)
		}
		return out
	}
	for text, want := range map[string]string{
		// Words before a sector may be anything, but the sector itself is kept with one locator.
		"PROXIMO A QR 308 CONJUNTO 13 LOTE 8": "PROXIMO A QR 308 CONJUNTO 13 LOTE 8|A QR 308 CONJUNTO 13 LOTE 8|QR 308 CONJUNTO 13 LOTE 8|308 CONJUNTO 13 LOTE 8|CONJUNTO 13 LOTE 8|" +
			"PROXIMO A QR 308 CONJUNTO 13|A QR 308 CONJUNTO 13|QR 308 CONJUNTO 13|308 CONJUNTO 13",
		"SQN 103 BLOCO B": "SQN 103 BLOCO B|103 BLOCO B",
		"QUADRA 20":       "QUADRA 20",
		// A street named stays.
		"CASA DO JOAO RUA 24 QUADRA 40 LOTE 14": "CASA JOAO RUA 24 QUADRA 40 LOTE 14|JOAO RUA 24 QUADRA 40 LOTE 14|RUA 24 QUADRA 40 LOTE 14|" +
			"CASA JOAO RUA 24 QUADRA 40|JOAO RUA 24 QUADRA 40|RUA 24 QUADRA 40",
	} {
		if got := strings.Join(keys(text), "|"); got != want {
			t.Errorf("%q:\n got %s\nwant %s", text, got, want)
		}
	}
}
