package bulk

import (
	"fmt"
	"strings"
)

// IdentityKey says which record fields decide that two enrolled faces are, or are not, the same
// person (docs/large-gallery-plan.md, Decision 4), since deployments differ in what their records
// carry. Each face's key is the first of the fields its record has:
//
//	fiscal      the document's fiscal number (identity_documents.fiscal_number)
//	name_birth  the register's name with its birth date, both present
//	person      the persons business key (persons.person_id), for importers whose person ids
//	            already mean one real person
//
// Two faces are the SAME identity when they share a document or a key. They are DIFFERENT when
// both have keys of the same field and the keys differ (a fiscal number never contradicts a name).
// Otherwise their relation is unknown, and they count on neither curve.
type IdentityKey []string

// DefaultIdentityKey is the fiscal number, then name with birth date.
var DefaultIdentityKey = IdentityKey{"fiscal", "name_birth"}

var identityFieldSQL = map[string]string{
	"fiscal":     `'fiscal:' || NULLIF(btrim(d.fiscal_number), '')`,
	"name_birth": `CASE WHEN NULLIF(btrim(r.name), '') IS NOT NULL AND NULLIF(btrim(r.birth_date), '') IS NOT NULL THEN 'name_birth:' || lower(btrim(r.name)) || '|' || btrim(r.birth_date) END`,
	"person":     `'person:' || p.person_id`,
}

// ParseIdentityKey reads a comma-separated field list.
func ParseIdentityKey(text string) (IdentityKey, error) {
	var out IdentityKey
	for _, f := range strings.Split(text, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if _, ok := identityFieldSQL[f]; !ok {
			return nil, fmt.Errorf("bulk: unknown identity field %q (fiscal, name_birth, person)", f)
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("bulk: no identity field given")
	}
	return out, nil
}

// keyExpr is the SQL for a face's key over identity_documents d, identity_registers r and
// persons p. Only fixed fragments from identityFieldSQL are spliced in.
func (k IdentityKey) keyExpr() string {
	parts := make([]string, len(k))
	for i, f := range k {
		parts[i] = identityFieldSQL[f]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "COALESCE(" + strings.Join(parts, ", ") + ")"
}

// labelsCTE is a CTE named labels: for every KNOWN embedding of type $1 in a MATCHED batch of
// model version $2, its id, document and identity key.
func (k IdentityKey) labelsCTE() string {
	return `labels AS (
		SELECT fe.id, d.id AS document_id, ` + k.keyExpr() + ` AS key
		FROM bulk_match_batch_members m
		JOIN bulk_match_batches bt ON bt.id = m.batch_id AND bt.status = 'MATCHED'
		     AND bt.embedding_type = $1 AND bt.model_version = $2
		JOIN feature_embeddings fe ON fe.id = m.feature_embedding_id
		JOIN biometric_features bf ON bf.id = fe.biometric_feature_id
		JOIN identity_files f ON f.id = bf.identity_file_id
		JOIN identity_registers r ON r.id = f.register_id
		JOIN identity_documents d ON d.id = r.document_id
		LEFT JOIN persons p ON p.id = d.person_id
	)`
}

// relationSQL classifies a pair of labels rows x and y as SAME, DIFFERENT or UNKNOWN.
const relationSQL = `CASE
	WHEN x.document_id = y.document_id OR x.key = y.key THEN 'SAME'
	WHEN x.key IS NOT NULL AND y.key IS NOT NULL AND split_part(x.key, ':', 1) = split_part(y.key, ':', 1) THEN 'DIFFERENT'
	ELSE 'UNKNOWN' END`

// graphIDSQL is a feature's graph id (graph.KnownFeatureID / graph.QuestionedFeatureID) from a
// biometric_features row aliased bf.
func graphIDSQL(bf string) string {
	return fmt.Sprintf(`CASE WHEN %[1]s.identity_file_id IS NOT NULL THEN %[1]s.identity_file_id::text
		ELSE 'TRACE:' || %[1]s.case_trace_id || '#feature' END`, bf)
}
