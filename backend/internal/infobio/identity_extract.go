package infobio

// IdentityFields holds whatever demographic fields could be confidently read from a search
// Entry. Every field is optional: the real InfoBio response's exact key names aren't confirmed
// anywhere in this codebase (Entry is an untyped map, and even the frontend hedges with multiple
// candidate spellings per field), so a field that matches nothing is left nil rather than guessed.
type IdentityFields struct {
	ContainerNumber *string
	RIN             *string
	Nome            *string
	DataNascimento  *string
	CPF             *string
	NomePai         *string
	NomeMae         *string
	NistPath        *string
}

// ExtractIdentityFields tries several known key spellings per field (mirroring
// frontend/src/services/infobio.ts's entryValue fallback) and leaves a field nil if none match.
// numeroIdentificacao is deliberately not extracted as its own field: it's the same value as the
// nif the search was performed with, not a distinct identifier InfoBio returns.
func ExtractIdentityFields(entry Entry) IdentityFields {
	return IdentityFields{
		ContainerNumber: firstMatch(entry, "container"),
		RIN:             firstMatch(entry, "RIN", "rin"),
		Nome:            firstMatch(entry, "NOME", "nomePessoa", "nome"),
		DataNascimento:  firstMatch(entry, "DT_NASCIMENTO", "dataNascimento", "data_nascimento", "dt_nascimento"),
		CPF:             firstMatch(entry, "CPF", "cpf"),
		NomePai:         firstMatch(entry, "PAI", "nomePai", "nome_pai", "pai"),
		NomeMae:         firstMatch(entry, "MAE", "nomeMae", "nome_mae", "mae"),
		// "caminhoNist" is host-relative (works directly with DownloadService.DownloadNIST /
		// INFOBIO_NIST_BASE_URL); "arquivo" is the UNC-style fallback InfoBio also returns.
		NistPath: firstMatch(entry, "caminhoNist", "arquivo"),
	}
}

func firstMatch(entry Entry, keys ...string) *string {
	for _, key := range keys {
		if value := stringValue(entry, key); value != "" {
			return &value
		}
	}
	return nil
}
