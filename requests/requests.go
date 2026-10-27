// Package requests holds what a case answers (case_requests, db/migrations/009_reports.sql): an
// authority asking the institute to examine material, with its questions (quesitos) and the files
// it sent in. A request is not the occurrence: it cites one (the occurrence or inquiry number), and
// Create links it when that number names exactly one occurrence. Every analysis answers a request
// (case_analyses.case_request_id); the laudo answers that request's questions. See
// docs/facial-reports-plan.md.
package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/occurrences"
)

// ValidationError is a request refused for its input.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// ErrCaseNotFound is returned for an unknown case.
var ErrCaseNotFound = errors.New("case not found")

const dateLayout = "2006-01-02"

// Input is a request as it is registered.
type Input struct {
	Authority         string   `json:"authority"`         // who asks, e.g. "37ª Delegacia de Polícia"
	DocumentReference string   `json:"documentReference"` // the request's own document, e.g. "Ofício nº 123/2026"
	DocumentDate      string   `json:"documentDate"`      // YYYY-MM-DD, optional
	ReceivedOn        string   `json:"receivedOn"`        // YYYY-MM-DD
	DueOn             string   `json:"dueOn"`             // YYYY-MM-DD, optional
	CitedReference    string   `json:"citedReference"`    // the occurrence or inquiry number it cites, optional
	OccurrenceID      *int64   `json:"occurrenceId"`      // the occurrence, when the caller already resolved it
	Notes             string   `json:"notes"`
	Questions         []string `json:"questions"` // the quesitos, in order
	FileIDs           []int64  `json:"fileIds"`   // evidence files of the case it sent in
}

// Request is a registered request.
type Request struct {
	ID                int64       `json:"id"`
	Sequence          int16       `json:"sequence"`
	Authority         string      `json:"authority"`
	DocumentReference string      `json:"documentReference"`
	DocumentDate      string      `json:"documentDate,omitempty"`
	ReceivedOn        string      `json:"receivedOn"`
	DueOn             string      `json:"dueOn,omitempty"`
	Overdue           bool        `json:"overdue"` // past its due date with an analysis still open
	CitedReference    string      `json:"citedReference,omitempty"`
	Occurrence        *Occurrence `json:"occurrence,omitempty"`
	Notes             string      `json:"notes,omitempty"`
	RegisteredBy      string      `json:"registeredBy,omitempty"`
	CreatedAt         time.Time   `json:"createdAt"`
	Questions         []string    `json:"questions"`
	Files             []File      `json:"files"`
}

// Occurrence is the occurrence a request was linked to, by its numbers.
type Occurrence struct {
	ID               int64  `json:"id"`
	Number           string `json:"number,omitempty"` // TrackID's "<year>.<seq>"
	Agency           string `json:"agency,omitempty"`
	OccurrenceNumber string `json:"occurrenceNumber,omitempty"`
}

// File is one evidence file a request sent in, with its SHA-256.
type File struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	SHA256      string    `json:"sha256"`
	ContentType string    `json:"contentType,omitempty"`
	SizeBytes   int64     `json:"sizeBytes"`
	AddedAt     time.Time `json:"addedAt"`
}

func parseDate(field, v string, required bool) (sql.NullTime, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		if required {
			return sql.NullTime{}, ValidationError(field + " is required")
		}
		return sql.NullTime{}, nil
	}
	t, err := time.Parse(dateLayout, v)
	if err != nil {
		return sql.NullTime{}, ValidationError(field + " must be a date (YYYY-MM-DD)")
	}
	return sql.NullTime{Time: t, Valid: true}, nil
}

func nullString(s string) sql.NullString {
	s = strings.TrimSpace(s)
	return sql.NullString{String: s, Valid: s != ""}
}

// Create registers a request on a case: its questions and the evidence files it sent in, and the
// occurrence it cites -- in.OccurrenceID, or else the one in.CitedReference names exactly (its
// TrackID number or the police number); an ambiguous or unknown number is kept as cited and left
// unlinked. When the case has no occurrence yet it is linked to that one too (same nature only,
// occurrences.LinkCase). The case's first request is what its analyses opened before it answer.
// registeredBy is the coordinator; checking that they are one is the caller's job.
func Create(ctx context.Context, sqlDB *sql.DB, caseID string, in Input, registeredBy int64) (Request, error) {
	authority, document := strings.TrimSpace(in.Authority), strings.TrimSpace(in.DocumentReference)
	if authority == "" {
		return Request{}, ValidationError("authority is required")
	}
	if document == "" {
		return Request{}, ValidationError("documentReference is required")
	}
	documentDate, err := parseDate("documentDate", in.DocumentDate, false)
	if err != nil {
		return Request{}, err
	}
	received, err := parseDate("receivedOn", in.ReceivedOn, true)
	if err != nil {
		return Request{}, err
	}
	due, err := parseDate("dueOn", in.DueOn, false)
	if err != nil {
		return Request{}, err
	}
	if due.Valid && due.Time.Before(received.Time) {
		return Request{}, ValidationError("dueOn cannot be before receivedOn")
	}
	var questions []string
	for _, q := range in.Questions {
		if q = strings.TrimSpace(q); q != "" {
			questions = append(questions, q)
		}
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	c, err := q.GetCaseOccurrenceID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, ErrCaseNotFound
	}
	if err != nil {
		return Request{}, err
	}
	if _, err := q.LockBiometricCase(ctx, c.ID); err != nil {
		return Request{}, err
	}

	occurrence := sql.NullInt64{}
	if in.OccurrenceID != nil {
		occurrence = sql.NullInt64{Int64: *in.OccurrenceID, Valid: true}
	} else if cited := strings.TrimSpace(in.CitedReference); cited != "" {
		ids, err := q.FindOccurrenceByReference(ctx, cited)
		if err != nil {
			return Request{}, err
		}
		if len(ids) == 1 {
			occurrence = sql.NullInt64{Int64: ids[0], Valid: true}
		}
	}
	if occurrence.Valid {
		natures, err := q.ListOccurrenceCaseTypes(ctx, occurrence.Int64)
		if err != nil {
			return Request{}, err
		}
		if len(natures) == 0 {
			return Request{}, ValidationError("occurrence not found")
		}
	}

	row, err := q.InsertCaseRequest(ctx, db.InsertCaseRequestParams{
		BiometricCaseID: c.ID, Authority: authority, DocumentReference: document, DocumentDate: documentDate,
		ReceivedOn: received.Time, DueOn: due, CitedReference: nullString(in.CitedReference), OccurrenceID: occurrence,
		Notes: nullString(in.Notes), RegisteredBy: sql.NullInt64{Int64: registeredBy, Valid: registeredBy != 0},
	})
	if err != nil {
		return Request{}, fmt.Errorf("requests: insert: %w", err)
	}
	for i, question := range questions {
		if err := q.InsertCaseRequestQuestion(ctx, db.InsertCaseRequestQuestionParams{CaseRequestID: row.ID, Sequence: int16(i + 1), Question: question}); err != nil {
			return Request{}, err
		}
	}
	if len(in.FileIDs) > 0 {
		n, err := q.AttachRequestFiles(ctx, db.AttachRequestFilesParams{CaseRequestID: row.ID, BiometricCaseID: c.ID, FileIds: in.FileIDs})
		if err != nil {
			return Request{}, err
		}
		if int(n) != len(in.FileIDs) {
			return Request{}, ValidationError("every file must be an evidence file of this case")
		}
	}
	if row.Sequence == 1 {
		if _, err := q.AttachUnrequestedAnalyses(ctx, db.AttachUnrequestedAnalysesParams{BiometricCaseID: c.ID, CaseRequestID: sql.NullInt64{Int64: row.ID, Valid: true}}); err != nil {
			return Request{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Request{}, err
	}
	if occurrence.Valid && !c.OccurrenceID.Valid {
		// Best effort: a case of another nature than the occurrence keeps the request's link only.
		var vErr occurrences.ValidationError
		if err := occurrences.LinkCase(ctx, sqlDB, caseID, &occurrence.Int64); err != nil && !errors.As(err, &vErr) {
			return Request{}, err
		}
	}
	list, err := List(ctx, sqlDB, caseID)
	if err != nil {
		return Request{}, err
	}
	for _, r := range list {
		if r.ID == row.ID {
			return r, nil
		}
	}
	return Request{}, fmt.Errorf("requests: %d not found after insert", row.ID)
}

// AttachFiles marks more of a case's evidence files as sent in by one of its requests.
func AttachFiles(ctx context.Context, sqlDB *sql.DB, caseID string, requestID int64, fileIDs []int64) error {
	q := db.New(sqlDB)
	c, err := q.GetCaseByCaseID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCaseNotFound
	}
	if err != nil {
		return err
	}
	if _, err := q.GetCaseRequestOfCase(ctx, db.GetCaseRequestOfCaseParams{ID: requestID, BiometricCaseID: c.ID}); errors.Is(err, sql.ErrNoRows) {
		return ValidationError("the request is not one of this case")
	} else if err != nil {
		return err
	}
	n, err := q.AttachRequestFiles(ctx, db.AttachRequestFilesParams{CaseRequestID: requestID, BiometricCaseID: c.ID, FileIds: fileIDs})
	if err != nil {
		return err
	}
	if int(n) != len(fileIDs) {
		return ValidationError("every file must be an evidence file of this case")
	}
	return nil
}

// List returns a case's requests in order, each with its questions and files. Overdue is set for
// a request past its due date while an analysis answering it is still open.
func List(ctx context.Context, sqlDB *sql.DB, caseID string) ([]Request, error) {
	return ListWith(ctx, db.New(sqlDB), caseID)
}

// ListWith is List on q, e.g. inside a transaction.
func ListWith(ctx context.Context, q *db.Queries, caseID string) ([]Request, error) {
	if _, err := q.GetCaseByCaseID(ctx, caseID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCaseNotFound
	} else if err != nil {
		return nil, err
	}
	rows, err := q.ListCaseRequests(ctx, caseID)
	if err != nil {
		return nil, err
	}
	questionRows, err := q.ListCaseRequestQuestions(ctx, caseID)
	if err != nil {
		return nil, err
	}
	fileRows, err := q.ListRequestFiles(ctx, caseID)
	if err != nil {
		return nil, err
	}
	analysisRows, err := q.ListCaseAnalyses(ctx, caseID)
	if err != nil {
		return nil, err
	}
	open := map[int64]bool{}
	for _, a := range analysisRows {
		if a.CaseRequestID.Valid && a.Status == "OPEN" {
			open[a.CaseRequestID.Int64] = true
		}
	}
	today := time.Now()
	out := make([]Request, len(rows))
	for i, r := range rows {
		req := Request{ID: r.ID, Sequence: r.Sequence, Authority: r.Authority, DocumentReference: r.DocumentReference,
			ReceivedOn: r.ReceivedOn.Format(dateLayout), CitedReference: r.CitedReference.String, Notes: r.Notes.String,
			RegisteredBy: r.RegisteredBy.String, CreatedAt: r.CreatedAt, Questions: []string{}, Files: []File{}}
		if r.DocumentDate.Valid {
			req.DocumentDate = r.DocumentDate.Time.Format(dateLayout)
		}
		if r.DueOn.Valid {
			req.DueOn = r.DueOn.Time.Format(dateLayout)
			req.Overdue = open[r.ID] && today.After(r.DueOn.Time.AddDate(0, 0, 1))
		}
		if r.OccurrenceID.Valid {
			o := &Occurrence{ID: r.OccurrenceID.Int64, Agency: r.Agency.String, OccurrenceNumber: r.OccurrenceNumber.String}
			if r.OccurrenceYear.Valid && r.OccurrenceSeq.Valid {
				o.Number = fmt.Sprintf("%04d.%04d", r.OccurrenceYear.Int32, r.OccurrenceSeq.Int32)
			}
			req.Occurrence = o
		}
		for _, qr := range questionRows {
			if qr.CaseRequestID == r.ID {
				req.Questions = append(req.Questions, qr.Question)
			}
		}
		for _, f := range fileRows {
			if f.CaseRequestID.Valid && f.CaseRequestID.Int64 == r.ID {
				req.Files = append(req.Files, File{ID: f.ID, Filename: f.Filename.String, SHA256: f.HashID.String,
					ContentType: f.ContentType.String, SizeBytes: f.SizeBytes.Int64, AddedAt: f.CreatedAt})
			}
		}
		out[i] = req
	}
	return out, nil
}
