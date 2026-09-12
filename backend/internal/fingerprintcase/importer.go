package fingerprintcase

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// ErrNotAFragment signals a case_id that doesn't match the case+evidência+latente+
// codificação shape (backend/internal/graph/migrations/004_fingerprint_case_indexes.cypher
// documents the convention). It is used by internal/graph's fingerprint-case bridge, kept
// deliberately unaware of graph sync from Import's perspective.
var ErrNotAFragment = errors.New("case id does not match the fragment shape required for graph sync")

// CaseTypeFingerprint is the criminal_cases.case_type value for every row this package imports:
// the LATENTES_SRMG dataset is exclusively fingerprint-comparison cases.
const CaseTypeFingerprint = "FINGERPRINT"

type Record struct {
	CaseID               string  `json:"caseId"`
	CaseType             string  `json:"caseType"`
	Description          string  `json:"description"`
	ResponsibleUser      *string `json:"responsibleUser,omitempty"`
	ComparisonType       *string `json:"comparisonType,omitempty"`
	RelatedReference     *string `json:"relatedReference,omitempty"`
	RelatedReferenceKind *string `json:"relatedReferenceKind,omitempty"`
}

type ImportStats struct {
	Read     int      `json:"read"`
	Inserted int      `json:"inserted"`
	Updated  int      `json:"updated"`
	Errors   []string `json:"errors,omitempty"`
}

func Parse(reader io.Reader, limit int) ([]Record, ImportStats, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, ImportStats{}, err
	}
	decodedReader := strings.NewReader(string(raw))
	if !utf8.Valid(raw) {
		decodedReader = strings.NewReader(readWindows1252(raw))
	}
	scanner := bufio.NewScanner(decodedReader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var records []Record
	var stats ImportStats
	for scanner.Scan() {
		stats.Read++
		line := strings.TrimSuffix(scanner.Text(), "\r")
		record, err := normalizeLine(line)
		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Sprintf("row %d: %v", stats.Read, err))
			continue
		}
		if limit > 0 && len(records) >= limit {
			break
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return records, stats, err
	}
	if len(stats.Errors) > 0 {
		return records, stats, fmt.Errorf("CSV contains %d invalid row(s)", len(stats.Errors))
	}
	return records, stats, nil
}

func normalizeLine(line string) (Record, error) {
	parts := strings.Split(line, ";")
	if len(parts) < 3 {
		return Record{}, fmt.Errorf("expected at least 3 columns, got %d", len(parts))
	}
	row := make([]string, 5)
	row[0] = parts[0]
	switch len(parts) {
	case 3:
		if !strings.EqualFold(strings.TrimSpace(parts[2]), "null") && strings.TrimSpace(parts[2]) != "" {
			return Record{}, errors.New("three-column rows must end with null")
		}
		row[1], row[2] = parts[1], parts[2]
	case 4:
		row[1], row[2], row[3] = parts[1], parts[2], parts[3]
	default:
		row[1] = strings.Join(parts[1:len(parts)-3], ";")
		copy(row[2:], parts[len(parts)-3:])
	}
	return normalizeRow(row)
}

func readWindows1252(raw []byte) string {
	reader := transform.NewReader(strings.NewReader(string(raw)), charmap.Windows1252.NewDecoder())
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return string(raw)
	}
	return string(decoded)
}

func normalizeRow(row []string) (Record, error) {
	if len(row) < 5 {
		return Record{}, fmt.Errorf("expected 5 columns, got %d", len(row))
	}
	caseID := strings.TrimSpace(row[0])
	description := strings.TrimSpace(row[1])
	if caseID == "" || description == "" {
		return Record{}, errors.New("case identifier and description are required")
	}
	record := Record{CaseID: caseID, CaseType: CaseTypeFingerprint, Description: description}
	record.ResponsibleUser = nullable(row[2])
	record.ComparisonType = nullable(row[3])
	record.RelatedReference = nullable(row[4])
	if record.RelatedReference != nil && record.ComparisonType != nil {
		comparison := strings.ToUpper(*record.ComparisonType)
		switch {
		// TP*  = a known ten-print (identified via InfoBio) searched against unsolved
		//        latents.
		// *TPF = an unsolved latent searched against the ten-print file (known persons'
		//        prints).
		// Either way the referenced side names an identified person. LT* against
		// anything else (ULF/ULP/PALM: other unsolved latents) names another criminal
		// case, not a person.
		case strings.HasPrefix(comparison, "TP"), strings.Contains(comparison, "TPF"):
			kind := "INFOBIO_NIF"
			record.RelatedReferenceKind = &kind
		case strings.HasPrefix(comparison, "LT"):
			kind := "CRIMINAL_CASE"
			record.RelatedReferenceKind = &kind
		}
	}
	return record, nil
}

func nullable(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "null") {
		return nil
	}
	return &value
}

// rowExecer is satisfied by both *sql.Tx and *sql.DB, so upsertRecord can run inside an ongoing
// transaction or, if ever needed, directly against the pool.
type rowExecer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// upsertRecord writes one record to criminal_cases, insert-or-update on case_id.
// RowsAffected() is always 1 for this statement, insert or update alike, so it can't tell the
// two apart. xmax = 0 can: Postgres leaves a freshly inserted row's xmax at 0, and the DO UPDATE
// branch always sets it, so RETURNING it distinguishes a real insert from a row that already
// existed.
func upsertRecord(ctx context.Context, execer rowExecer, record Record) (bool, error) {
	var inserted bool
	err := execer.QueryRowContext(ctx, `
INSERT INTO criminal_cases
	(case_id, case_type, description, responsible_user, comparison_type, related_reference, related_reference_kind)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (case_id) DO UPDATE SET
	case_type = EXCLUDED.case_type,
	description = EXCLUDED.description,
	responsible_user = EXCLUDED.responsible_user,
	comparison_type = EXCLUDED.comparison_type,
	related_reference = EXCLUDED.related_reference,
	related_reference_kind = EXCLUDED.related_reference_kind,
	updated_at = NOW()
RETURNING (xmax = 0)`, record.CaseID, record.CaseType, record.Description, record.ResponsibleUser, record.ComparisonType, record.RelatedReference, record.RelatedReferenceKind).Scan(&inserted)
	return inserted, err
}

func Import(ctx context.Context, db *sql.DB, reader io.Reader) (ImportStats, error) {
	if db == nil {
		return ImportStats{}, errors.New("database is not configured")
	}
	records, stats, parseErr := Parse(reader, 0)
	if parseErr != nil {
		return stats, parseErr
	}
	transaction, err := db.BeginTx(ctx, nil)
	if err != nil {
		return stats, err
	}
	defer transaction.Rollback()
	for _, record := range records {
		inserted, err := upsertRecord(ctx, transaction, record)
		if err != nil {
			return stats, err
		}
		if inserted {
			stats.Inserted++
		} else {
			stats.Updated++
		}
	}
	if err := transaction.Commit(); err != nil {
		return stats, err
	}
	return stats, nil
}
