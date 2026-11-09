package calibration

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// IdentitySource samples enrolled faces from the identity tables (persons, identity_documents, identity_registers,
// identity_files), with the truth a deployment trusts:
//
//	person      persons.person_id: for importers whose person ids already mean one real person (for example one
//	            deduplicated by fingerprints)
//	fiscal      identity_documents.fiscal_number
//	name_birth  the register's name with its birth date, both present
//
// Key lists the fields in order; a face's identity is the first it has, and a face with none is left out.
// DocumentTypes, when given, keeps only those documents' faces: the population whose records are trusted. Faces
// withdrawn from matching are left out.
type IdentitySource struct {
	DB            *sql.DB
	EmbeddingType string
	Key           []string
	DocumentTypes []string
}

var identityFields = map[string]string{
	"person":     `'person:' || p.person_id`,
	"fiscal":     `'fiscal:' || NULLIF(btrim(d.fiscal_number), '')`,
	"name_birth": `CASE WHEN NULLIF(btrim(r.name), '') IS NOT NULL AND NULLIF(btrim(r.birth_date), '') IS NOT NULL THEN 'name_birth:' || lower(btrim(r.name)) || '|' || btrim(r.birth_date) END`,
}

// ParseKey reads a comma-separated list of identity fields.
func ParseKey(text string) ([]string, error) {
	var key []string
	for _, field := range strings.Split(text, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := identityFields[field]; !ok {
			return nil, fmt.Errorf("calibration: unknown identity field %q (person, fiscal, name_birth)", field)
		}
		key = append(key, field)
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("calibration: no identity field given")
	}
	return key, nil
}

func (s IdentitySource) keyExpr() (string, error) {
	if len(s.Key) == 0 {
		return "", fmt.Errorf("calibration: no identity field given")
	}
	parts := make([]string, len(s.Key))
	for i, field := range s.Key {
		expr, ok := identityFields[field]
		if !ok {
			return "", fmt.Errorf("calibration: unknown identity field %q", field)
		}
		parts[i] = expr // only fixed fragments are spliced in
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return "COALESCE(" + strings.Join(parts, ", ") + ")", nil
}

// Describe says what the source trusts.
func (s IdentitySource) Describe() string {
	documents := "every document type"
	if len(s.DocumentTypes) > 0 {
		documents = "document types " + strings.Join(s.DocumentTypes, ", ")
	}
	return fmt.Sprintf("identity tables, %s faces, one person per %s, %s", s.EmbeddingType, strings.Join(s.Key, " then "), documents)
}

// Sample draws identities with at least two faces, the same ones for the same seed.
func (s IdentitySource) Sample(ctx context.Context, identities, maxFaces int, seed int64) ([]Face, error) {
	key, err := s.keyExpr()
	if err != nil {
		return nil, err
	}
	types := s.DocumentTypes
	if types == nil {
		types = []string{}
	}
	rows, err := s.DB.QueryContext(ctx, `
		WITH faces AS (
			SELECT fe.id, fe.embedding::text AS vector, `+key+` AS key
			FROM feature_embeddings fe
			JOIN biometric_features bf ON bf.id = fe.biometric_feature_id AND bf.withdrawn_at IS NULL
			JOIN identity_files f ON f.id = bf.identity_file_id
			JOIN identity_registers r ON r.id = f.register_id
			JOIN identity_documents d ON d.id = r.document_id
			LEFT JOIN persons p ON p.id = d.person_id
			WHERE fe.embedding_type = $1 AND (cardinality($2::text[]) = 0 OR d.document_type = ANY($2::text[]))
		), keyed AS (
			SELECT * FROM faces WHERE key IS NOT NULL
		), chosen AS (
			SELECT key FROM keyed GROUP BY key HAVING COUNT(*) >= 2 ORDER BY md5(key || $3) LIMIT $4
		), ranked AS (
			SELECT k.id, k.key, k.vector, row_number() OVER (PARTITION BY k.key ORDER BY k.id) AS n
			FROM keyed k JOIN chosen USING (key)
		)
		SELECT id, key, vector FROM ranked WHERE n <= $5 ORDER BY key, id`,
		s.EmbeddingType, types, strconv.FormatInt(seed, 10), identities, maxFaces)
	if err != nil {
		return nil, fmt.Errorf("calibration: sample: %w", err)
	}
	defer rows.Close()
	var faces []Face
	for rows.Next() {
		var face Face
		var vector string
		if err := rows.Scan(&face.ID, &face.Identity, &vector); err != nil {
			return nil, err
		}
		if face.Vector, err = parseVector(vector); err != nil {
			return nil, fmt.Errorf("calibration: embedding %d: %w", face.ID, err)
		}
		faces = append(faces, face)
	}
	return faces, rows.Err()
}

// parseVector reads pgvector's text form, "[v1,v2,...]".
func parseVector(text string) ([]float32, error) {
	parts := strings.Split(strings.Trim(text, "[] "), ",")
	out := make([]float32, len(parts))
	for i, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return nil, err
		}
		out[i] = float32(v)
	}
	return out, nil
}
