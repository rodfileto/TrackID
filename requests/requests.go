// Package requests holds what cases answer (case_requests, db/migrations/004_requests.sql): an
// authority asking the institute to examine material, with its questions (quesitos) and the files
// it sent in. A request comes first: Register records it, finds or creates the occurrence it cites
// and opens or joins its cases (case_request_cases, 005_cases.sql), one per exam asked.
// Create adds a later request to a case already open. A request is not the occurrence: it cites
// one, by the police agency's number or TrackID's, and its inquiry (IPL) by the inquiry's number
// (occurrences.inquiry_number, 003_occurrences.sql). Every analysis answers a request (case_analyses.case_request_id);
// the laudo answers that request's questions. See docs/facial-reports-plan.md, "Request first".
package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/cases"
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
	Authority         string   `json:"authority"`          // who asks, e.g. "37ª Delegacia de Polícia"
	RequestingUnitID  *int64   `json:"requestingUnitId"`   // the unit that asks, when it is one we know, of any organization; optional
	DocumentReference string   `json:"documentReference"`  // the request's own document, e.g. "Ofício nº 123/2026"
	DocumentDate      string   `json:"documentDate"`       // YYYY-MM-DD, optional
	ReceivedOn        string   `json:"receivedOn"`         // YYYY-MM-DD
	DueOn             string   `json:"dueOn"`              // YYYY-MM-DD, optional
	CitedReference    string   `json:"citedReference"`     // the occurrence number it cites (TrackID's or the police report's), optional
	CitedInquiry      string   `json:"citedInquiryNumber"` // the inquiry (IPL) number it cites, optional
	OccurrenceID      *int64   `json:"occurrenceId"`       // the occurrence, when the caller already resolved it
	Notes             string   `json:"notes"`
	Questions         []string `json:"questions"` // the quesitos, in order
	FileIDs           []int64  `json:"fileIds"`   // evidence files of the case it sent in (Create only)
	// References are other numbers its document goes by, labelled by the institute (Reference).
	References []Reference `json:"references"`
	Task       string      `json:"task"` // the task it asks of the case, an analysis type (Create only)
}

// Reference is another number a request's document goes by in the systems around it, such as a task
// in a case-management system or a process in a document system. Label names the system.
type Reference struct {
	Label  string `json:"label"`
	Number string `json:"number"`
}

// RegisterInput is a request as it arrives, with what it opens: the occurrence it cites, when
// TrackID does not have it yet, and its cases.
type RegisterInput struct {
	Input
	// Occurrence is the occurrence to create when the request cites one TrackID does not have
	// (Input.OccurrenceID, Input.CitedReference and Input.CitedInquiry find existing ones). Its
	// CaseID is ignored.
	Occurrence *occurrences.CreateInput
	// CaseType is the nature of new cases when there is no occurrence (a civil request may have
	// none); with an occurrence, its nature is theirs.
	CaseType string
	Cases    []CaseInput
	// ConcludedAt marks a request that was already answered when registered (an import of old
	// reports): its analyses are recorded closed at this moment instead of opened for work, and may
	// be of a type not yet offered. Importers only: the API never sets it.
	ConcludedAt *time.Time `json:"-"`
}

// CaseInput is one case a request opens (Description) or joins (CaseID), and the task it asks of
// it: an analysis type (analyses.Type), chosen by the coordinator. A new case takes the task's
// modality; an existing one must be of it.
type CaseInput struct {
	CaseID      string `json:"caseId"`
	Task        string `json:"task"`
	Description string `json:"description"`
}

// Registered is what registering a request made.
type Registered struct {
	RequestID  int64        `json:"requestId"`
	Occurrence *Occurrence  `json:"occurrence,omitempty"`
	Cases      []cases.Case `json:"cases"`
}

// CaseRef is a case a request opened or joined.
type CaseRef struct {
	CaseID   string `json:"caseId"`
	Modality string `json:"modality"`
}

// Summary is a request in the list of every request.
type Summary struct {
	ID                int64       `json:"id"`
	Authority         string      `json:"authority"`
	DocumentReference string      `json:"documentReference"`
	ReceivedOn        string      `json:"receivedOn"`
	DueOn             string      `json:"dueOn,omitempty"`
	OpenAnalyses      int64       `json:"openAnalyses"`
	Overdue           bool        `json:"overdue"`
	CitedReference    string      `json:"citedReference,omitempty"`
	CitedInquiry      string      `json:"citedInquiryNumber,omitempty"`
	OccurrenceNumber  string      `json:"occurrenceNumber,omitempty"` // TrackID's, else the agency's
	CaseIDs           []string    `json:"caseIds"`
	Tasks             []string    `json:"tasks"` // the analyses it asked, by type, across its cases
	References        []Reference `json:"references"`
}

// Request is a registered request.
type Request struct {
	ID                int64       `json:"id"`
	Sequence          int16       `json:"sequence"`
	Authority         string      `json:"authority"`
	RequestingUnitID  *int64      `json:"requestingUnitId,omitempty"`
	DocumentReference string      `json:"documentReference"`
	DocumentDate      string      `json:"documentDate,omitempty"`
	ReceivedOn        string      `json:"receivedOn"`
	DueOn             string      `json:"dueOn,omitempty"`
	Overdue           bool        `json:"overdue"` // past its due date with an analysis still open
	CitedReference    string      `json:"citedReference,omitempty"`
	CitedInquiry      string      `json:"citedInquiryNumber,omitempty"`
	Occurrence        *Occurrence `json:"occurrence,omitempty"`
	Notes             string      `json:"notes,omitempty"`
	RegisteredBy      string      `json:"registeredBy,omitempty"`
	CreatedAt         time.Time   `json:"createdAt"`
	Questions         []string    `json:"questions"`
	Files             []File      `json:"files"`
	Cases             []CaseRef   `json:"cases"` // every case it opened or joined, this one included
	Tasks             []string    `json:"tasks"` // the analyses it asked of this case, by type
	References        []Reference `json:"references"`
}

// Occurrence is the occurrence a request was linked to, by its numbers.
type Occurrence struct {
	ID               int64  `json:"id"`
	Number           string `json:"number,omitempty"` // TrackID's "<year>.<seq>"
	Agency           string `json:"agency,omitempty"`
	OccurrenceNumber string `json:"occurrenceNumber,omitempty"`
	InquiryNumber    string `json:"inquiryNumber,omitempty"`
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

// maxAttempts bounds how often Register retries when a concurrent create took the number it gave
// a new case or occurrence.
const maxAttempts = 3

// fields is a request's own fields, checked.
type fields struct {
	authority, document         string
	documentDate, received, due sql.NullTime
	questions                   []string
	references                  []Reference
}

func check(in Input) (fields, error) {
	f := fields{authority: strings.TrimSpace(in.Authority), document: strings.TrimSpace(in.DocumentReference)}
	if f.authority == "" {
		return f, ValidationError("authority is required")
	}
	if f.document == "" {
		return f, ValidationError("documentReference is required")
	}
	var err error
	if f.documentDate, err = parseDate("documentDate", in.DocumentDate, false); err != nil {
		return f, err
	}
	if f.received, err = parseDate("receivedOn", in.ReceivedOn, true); err != nil {
		return f, err
	}
	if f.due, err = parseDate("dueOn", in.DueOn, false); err != nil {
		return f, err
	}
	if f.due.Valid && f.due.Time.Before(f.received.Time) {
		return f, ValidationError("dueOn cannot be before receivedOn")
	}
	for _, q := range in.Questions {
		if q = strings.TrimSpace(q); q != "" {
			f.questions = append(f.questions, q)
		}
	}
	for _, r := range in.References {
		r = Reference{Label: strings.TrimSpace(r.Label), Number: strings.TrimSpace(r.Number)}
		if r.Label == "" || r.Number == "" {
			return f, ValidationError("a reference needs a label and a number")
		}
		if !slices.Contains(f.references, r) {
			f.references = append(f.references, r)
		}
	}
	return f, nil
}

// Register records a request as it arrives and what it opens, in one transaction: the occurrence
// it cites -- in.OccurrenceID, else the one in.CitedReference names exactly (TrackID's number or
// the police number) or in.CitedInquiry does (the inquiry number), else in.Occurrence created
// now; an unknown cited number with no occurrence to create is kept as cited, unlinked -- and its
// cases, new ones numbered and opened with their automatic analyses (which answer this request),
// existing ones joined. A case with no occurrence is linked to the request's when their nature
// agrees. registeredBy is the coordinator; checking that they are one is the caller's job.
func Register(ctx context.Context, sqlDB *sql.DB, in RegisterInput, registeredBy int64, now time.Time) (Registered, error) {
	f, err := check(in.Input)
	if err != nil {
		return Registered{}, err
	}
	if len(in.Cases) == 0 {
		return Registered{}, ValidationError("a request opens or joins at least one case")
	}
	if len(in.FileIDs) > 0 {
		return Registered{}, ValidationError("attach files after registering, to the case they belong to")
	}
	for attempt := 1; ; attempt++ {
		out, err := register(ctx, sqlDB, in, f, registeredBy, now)
		if err == nil || attempt >= maxAttempts || !(cases.IsDuplicateNumber(err) || occurrences.IsDuplicateNumber(err)) {
			return out, err
		}
	}
}

func register(ctx context.Context, sqlDB *sql.DB, in RegisterInput, f fields, registeredBy int64, now time.Time) (Registered, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Registered{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)

	occurrence, err := resolveOccurrence(ctx, q, in, now)
	if err != nil {
		return Registered{}, err
	}
	nature := strings.ToUpper(strings.TrimSpace(in.CaseType))
	if occurrence.Valid {
		natures, err := q.ListOccurrenceCaseTypes(ctx, occurrence.Int64)
		if err != nil {
			return Registered{}, err
		}
		if len(natures) == 0 {
			return Registered{}, ValidationError("occurrence not found")
		}
		nature = natures[0]
	}

	type opened struct {
		id   int64
		c    cases.Case
		task analyses.Type
	}
	var list []opened
	seen := map[string]bool{}
	for _, ci := range in.Cases {
		task, err := parseTask(ci.Task)
		if err == nil && in.ConcludedAt == nil {
			err = checkOffered(task, ci.Task)
		}
		if err != nil {
			return Registered{}, err
		}
		o := opened{task: task}
		if id := strings.TrimSpace(ci.CaseID); id != "" {
			row, err := q.GetCaseForRequest(ctx, id)
			if errors.Is(err, sql.ErrNoRows) {
				return Registered{}, ValidationError("case " + id + " not found")
			}
			if err != nil {
				return Registered{}, err
			}
			o.id, o.c = row.ID, cases.Case{CaseID: row.CaseID, CaseType: row.CaseType, Modality: row.Modality, Description: row.Description}
			if o.c.Modality != string(task.Modality()) {
				return Registered{}, ValidationError("case " + id + " is not of the task's modality")
			}
		} else {
			if nature == "" {
				return Registered{}, ValidationError("caseType is required for a new case when the request has no occurrence")
			}
			c, id, err := cases.CreateWith(ctx, q, cases.CreateInput{CaseType: nature, Modality: string(task.Modality()), Description: ci.Description})
			var vErr cases.ValidationError
			if errors.As(err, &vErr) {
				return Registered{}, ValidationError(vErr.Error())
			}
			if err != nil {
				return Registered{}, err
			}
			o.id, o.c = id, c
		}
		if seen[o.c.CaseID] {
			return Registered{}, ValidationError("case " + o.c.CaseID + " is listed twice")
		}
		seen[o.c.CaseID] = true
		list = append(list, o)
	}

	requestID, err := insertRequest(ctx, q, in.Input, f, occurrence, registeredBy)
	if err != nil {
		return Registered{}, err
	}
	out := Registered{RequestID: requestID, Cases: []cases.Case{}}
	for _, o := range list {
		if err := joinCase(ctx, q, requestID, o.id, o.c.CaseType, occurrence); err != nil {
			return Registered{}, err
		}
		if in.ConcludedAt != nil {
			_, err = analyses.OpenConcludedIn(ctx, q, o.id, o.c.Modality, o.task, requestID, registeredBy, *in.ConcludedAt)
		} else {
			_, err = analyses.OpenIn(ctx, q, o.id, o.c.Modality, o.task, requestID, registeredBy)
		}
		if err != nil {
			return Registered{}, err
		}
		out.Cases = append(out.Cases, o.c)
	}
	if occurrence.Valid {
		if out.Occurrence, err = occurrenceRef(ctx, q, occurrence.Int64); err != nil {
			return Registered{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Registered{}, err
	}
	return out, nil
}

// resolveOccurrence finds the occurrence a request cites, or creates in.Occurrence.
func resolveOccurrence(ctx context.Context, q *db.Queries, in RegisterInput, now time.Time) (sql.NullInt64, error) {
	if in.OccurrenceID != nil {
		return sql.NullInt64{Int64: *in.OccurrenceID, Valid: true}, nil
	}
	var byNumber, byInquiry []int64
	var err error
	if cited := strings.TrimSpace(in.CitedReference); cited != "" {
		if byNumber, err = q.FindOccurrenceByReference(ctx, cited); err != nil {
			return sql.NullInt64{}, err
		}
	}
	if cited := strings.TrimSpace(in.CitedInquiry); cited != "" {
		if byInquiry, err = q.FindOccurrenceByInquiryNumber(ctx, cited); err != nil {
			return sql.NullInt64{}, err
		}
	}
	switch {
	case len(byNumber) == 1 && len(byInquiry) == 1 && byNumber[0] != byInquiry[0]:
		return sql.NullInt64{}, ValidationError("the cited occurrence number and inquiry number name different occurrences")
	case len(byNumber) == 1:
		return sql.NullInt64{Int64: byNumber[0], Valid: true}, nil
	case len(byInquiry) == 1:
		return sql.NullInt64{Int64: byInquiry[0], Valid: true}, nil
	}
	if in.Occurrence == nil {
		return sql.NullInt64{}, nil
	}
	o := *in.Occurrence
	o.CaseID = ""
	created, err := occurrences.CreateWith(ctx, q, o, now)
	var vErr occurrences.ValidationError
	if errors.As(err, &vErr) {
		return sql.NullInt64{}, ValidationError(vErr.Error())
	}
	if err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: created.ID, Valid: true}, nil
}

func occurrenceRef(ctx context.Context, q *db.Queries, id int64) (*Occurrence, error) {
	o, err := q.GetOccurrenceNumbers(ctx, id)
	if err != nil {
		return nil, err
	}
	ref := &Occurrence{ID: id, Agency: o.Agency.String, OccurrenceNumber: o.OccurrenceNumber.String, InquiryNumber: o.InquiryNumber.String}
	if o.OccurrenceYear.Valid && o.OccurrenceSeq.Valid {
		ref.Number = fmt.Sprintf("%04d.%04d", o.OccurrenceYear.Int32, o.OccurrenceSeq.Int32)
	}
	return ref, nil
}

func insertRequest(ctx context.Context, q *db.Queries, in Input, f fields, occurrence sql.NullInt64, registeredBy int64) (int64, error) {
	cited := strings.TrimSpace(in.CitedReference)
	requestingUnit := sql.NullInt64{}
	if in.RequestingUnitID != nil {
		requestingUnit = sql.NullInt64{Int64: *in.RequestingUnitID, Valid: true}
	}
	id, err := q.InsertCaseRequest(ctx, db.InsertCaseRequestParams{
		RequestingUnitID: requestingUnit,
		Authority:        f.authority, DocumentReference: f.document, DocumentDate: f.documentDate,
		ReceivedOn: f.received.Time, DueOn: f.due, CitedReference: nullString(cited),
		CitedInquiryNumber: nullString(in.CitedInquiry), OccurrenceID: occurrence,
		Notes: nullString(in.Notes), RegisteredBy: sql.NullInt64{Int64: registeredBy, Valid: registeredBy != 0},
	})
	if err != nil && strings.Contains(err.Error(), "case_requests_requesting_unit_id_fkey") {
		return 0, ValidationError("requesting unit not found")
	}
	if err != nil {
		return 0, fmt.Errorf("requests: insert: %w", err)
	}
	for i, question := range f.questions {
		if err := q.InsertCaseRequestQuestion(ctx, db.InsertCaseRequestQuestionParams{CaseRequestID: id, Sequence: int16(i + 1), Question: question}); err != nil {
			return 0, err
		}
	}
	for _, r := range f.references {
		if err := q.InsertCaseRequestReference(ctx, db.InsertCaseRequestReferenceParams{CaseRequestID: id, Label: r.Label, Number: r.Number}); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// taskOf reads a task a coordinator chose: an analysis type that can be opened now.
func taskOf(task string) (analyses.Type, error) {
	t, err := parseTask(task)
	if err != nil {
		return "", err
	}
	return t, checkOffered(t, task)
}

func parseTask(task string) (analyses.Type, error) {
	t := analyses.Type(strings.ToUpper(strings.TrimSpace(task)))
	switch {
	case t == "":
		return "", ValidationError("each case needs its task: the examination the request asks")
	case !t.Valid():
		return "", ValidationError("unknown task " + task)
	}
	return t, nil
}

func checkOffered(t analyses.Type, task string) error {
	if !t.Offered() {
		return ValidationError("the " + task + " examination is not available yet")
	}
	return nil
}

// joinCase makes the request the case's next one. The case's first request is what its analyses
// opened before it answer, and a case with no occurrence takes the request's when their nature
// agrees (otherwise only the request keeps the link).
func joinCase(ctx context.Context, q *db.Queries, requestID, caseID int64, caseType string, occurrence sql.NullInt64) error {
	if _, err := q.LockBiometricCase(ctx, caseID); err != nil {
		return err
	}
	sequence, err := q.LinkCaseRequest(ctx, db.LinkCaseRequestParams{CaseRequestID: requestID, BiometricCaseID: caseID})
	if err != nil {
		return fmt.Errorf("requests: join case: %w", err)
	}
	if sequence == 1 {
		if _, err := q.AttachUnrequestedAnalyses(ctx, db.AttachUnrequestedAnalysesParams{BiometricCaseID: caseID, CaseRequestID: sql.NullInt64{Int64: requestID, Valid: true}}); err != nil {
			return err
		}
	}
	if !occurrence.Valid {
		return nil
	}
	natures, err := q.ListOccurrenceCaseTypes(ctx, occurrence.Int64)
	if err != nil {
		return err
	}
	if !slices.Contains(natures, caseType) {
		return nil
	}
	_, err = q.SetBiometricCaseOccurrenceIfNone(ctx, db.SetBiometricCaseOccurrenceIfNoneParams{ID: caseID, OccurrenceID: occurrence})
	return err
}

// Create registers a later request on a case already open: its questions, the evidence files of
// the case it sent in, the occurrence it cites, found as Register finds it (never created), and
// the task it asks (in.Task), opened as an analysis answering it.
// registeredBy is the coordinator; checking that they are one is the caller's job.
func Create(ctx context.Context, sqlDB *sql.DB, caseID string, in Input, registeredBy int64) (Request, error) {
	f, err := check(in)
	if err != nil {
		return Request{}, err
	}
	task, err := taskOf(in.Task)
	if err != nil {
		return Request{}, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	c, err := q.GetCaseForRequest(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, ErrCaseNotFound
	}
	if err != nil {
		return Request{}, err
	}
	occurrence, err := resolveOccurrence(ctx, q, RegisterInput{Input: in}, time.Now())
	if err != nil {
		return Request{}, err
	}
	if occurrence.Valid {
		if natures, err := q.ListOccurrenceCaseTypes(ctx, occurrence.Int64); err != nil {
			return Request{}, err
		} else if len(natures) == 0 {
			return Request{}, ValidationError("occurrence not found")
		}
	}
	requestID, err := insertRequest(ctx, q, in, f, occurrence, registeredBy)
	if err != nil {
		return Request{}, err
	}
	if err := joinCase(ctx, q, requestID, c.ID, c.CaseType, occurrence); err != nil {
		return Request{}, err
	}
	if c.Modality != string(task.Modality()) {
		return Request{}, ValidationError("the case is not of the task's modality")
	}
	if _, err := analyses.OpenIn(ctx, q, c.ID, c.Modality, task, requestID, registeredBy); err != nil {
		return Request{}, err
	}
	if len(in.FileIDs) > 0 {
		n, err := q.AttachRequestFiles(ctx, db.AttachRequestFilesParams{CaseRequestID: requestID, BiometricCaseID: c.ID, FileIds: in.FileIDs})
		if err != nil {
			return Request{}, err
		}
		if int(n) != len(in.FileIDs) {
			return Request{}, ValidationError("every file must be an evidence file of this case")
		}
	}
	if err := tx.Commit(); err != nil {
		return Request{}, err
	}
	list, err := List(ctx, sqlDB, caseID)
	if err != nil {
		return Request{}, err
	}
	for _, r := range list {
		if r.ID == requestID {
			return r, nil
		}
	}
	return Request{}, fmt.Errorf("requests: %d not found after insert", requestID)
}

// UpdateInput is what can be corrected in a registered request, the whole of it each time. The
// occurrence is found from CitedInquiry as Register finds it: named exactly by one, or none.
type UpdateInput struct {
	Authority         string
	DocumentReference string
	DocumentDate      string
	ReceivedOn        string
	DueOn             string
	CitedInquiry      string
	OccurrenceID      *int64 // the occurrence, when the caller knows it; otherwise CitedInquiry finds it
	Notes             string
	References        []Reference
}

// Update corrects a registered request. Its cases and analyses stay as they are (MoveToCase moves it).
func Update(ctx context.Context, sqlDB *sql.DB, requestID int64, in UpdateInput) error {
	f, err := check(Input{Authority: in.Authority, DocumentReference: in.DocumentReference, DocumentDate: in.DocumentDate,
		ReceivedOn: in.ReceivedOn, DueOn: in.DueOn, References: in.References})
	if err != nil {
		return err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	occurrence, err := resolveOccurrence(ctx, q, RegisterInput{Input: Input{OccurrenceID: in.OccurrenceID, CitedInquiry: in.CitedInquiry}}, time.Now())
	if err != nil {
		return err
	}
	rows, err := q.UpdateCaseRequest(ctx, db.UpdateCaseRequestParams{
		ID: requestID, Authority: f.authority, DocumentReference: f.document, DocumentDate: f.documentDate,
		ReceivedOn: f.received.Time, DueOn: f.due, CitedInquiryNumber: nullString(in.CitedInquiry),
		OccurrenceID: occurrence, Notes: nullString(in.Notes),
	})
	if err != nil {
		return fmt.Errorf("requests: update %d: %w", requestID, err)
	}
	if rows == 0 {
		return ValidationError("request not found")
	}
	if err := q.DeleteCaseRequestReferences(ctx, requestID); err != nil {
		return err
	}
	for _, r := range f.references {
		if err := q.InsertCaseRequestReference(ctx, db.InsertCaseRequestReferenceParams{CaseRequestID: requestID, Label: r.Label, Number: r.Number}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MoveToCase moves a request from one case to another of the same modality, with the analyses that
// answer it and the material it sent in, when it was registered under the wrong case.
func MoveToCase(ctx context.Context, sqlDB *sql.DB, requestID int64, fromCaseID, toCaseID string) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	from, err := q.GetCaseForRequest(ctx, fromCaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return ValidationError("case " + fromCaseID + " not found")
	}
	if err != nil {
		return err
	}
	to, err := q.GetCaseForRequest(ctx, toCaseID)
	if errors.Is(err, sql.ErrNoRows) {
		return ValidationError("case " + toCaseID + " not found")
	}
	if err != nil {
		return err
	}
	if from.ID == to.ID {
		return nil
	}
	if from.Modality != to.Modality {
		return ValidationError("the cases are of different modalities")
	}
	// In a fixed order, so two moves in opposite directions can't deadlock.
	first, second := min(from.ID, to.ID), max(from.ID, to.ID)
	for _, id := range []int64{first, second} {
		if _, err := q.LockBiometricCase(ctx, id); err != nil {
			return err
		}
	}
	rows, err := q.MoveCaseRequestLink(ctx, db.MoveCaseRequestLinkParams{ToCase: to.ID, CaseRequestID: requestID, FromCase: from.ID})
	if err != nil {
		return fmt.Errorf("requests: move %d: %w", requestID, err)
	}
	if rows == 0 {
		return ValidationError("the request is not on case " + fromCaseID)
	}
	if _, err := q.MoveRequestAnalyses(ctx, db.MoveRequestAnalysesParams{ToCase: to.ID, CaseRequestID: requestID, FromCase: from.ID}); err != nil {
		return fmt.Errorf("requests: move analyses of %d: %w", requestID, err)
	}
	if err := q.MoveRequestFiles(ctx, db.MoveRequestFilesParams{ToCase: to.ID, CaseRequestID: requestID, FromCase: from.ID}); err != nil {
		return fmt.Errorf("requests: move files of %d: %w", requestID, err)
	}
	return tx.Commit()
}

// ListAll returns up to limit requests across every case, soonest due first (no due date last),
// each with its cases and whether it is overdue.
func ListAll(ctx context.Context, sqlDB *sql.DB, reference string, limit int) ([]Summary, error) {
	rows, err := db.New(sqlDB).ListRequests(ctx, db.ListRequestsParams{Reference: nullString(reference), RowLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	today := time.Now()
	out := make([]Summary, len(rows))
	for i, r := range rows {
		s := Summary{ID: r.ID, Authority: r.Authority, DocumentReference: r.DocumentReference,
			ReceivedOn: r.ReceivedOn.Format(dateLayout), OpenAnalyses: r.OpenAnalyses,
			CitedReference: r.CitedReference.String, CitedInquiry: r.CitedInquiryNumber.String, CaseIDs: r.CaseIds, Tasks: r.Tasks}
		if s.CaseIDs == nil {
			s.CaseIDs = []string{}
		}
		if s.Tasks == nil {
			s.Tasks = []string{}
		}
		if err := json.Unmarshal(r.Refs, &s.References); err != nil {
			return nil, fmt.Errorf("requests: references: %w", err)
		}
		if r.DueOn.Valid {
			s.DueOn = r.DueOn.Time.Format(dateLayout)
			s.Overdue = r.OpenAnalyses > 0 && today.After(r.DueOn.Time.AddDate(0, 0, 1))
		}
		if r.OccurrenceYear.Valid && r.OccurrenceSeq.Valid {
			s.OccurrenceNumber = fmt.Sprintf("%04d.%04d", r.OccurrenceYear.Int32, r.OccurrenceSeq.Int32)
		} else if r.OccurrenceNumber.Valid {
			s.OccurrenceNumber = r.Agency.String + " " + r.OccurrenceNumber.String
		}
		out[i] = s
	}
	return out, nil
}

// Check reports whether requestID is one of caseID's requests: ErrCaseNotFound for an unknown
// case, a ValidationError for a request of another case.
func Check(ctx context.Context, sqlDB *sql.DB, caseID string, requestID int64) error {
	q := db.New(sqlDB)
	c, err := q.GetCaseByCaseID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCaseNotFound
	}
	if err != nil {
		return err
	}
	if _, err := q.GetCaseRequestOfCase(ctx, db.GetCaseRequestOfCaseParams{CaseRequestID: requestID, BiometricCaseID: c.ID}); errors.Is(err, sql.ErrNoRows) {
		return ValidationError("the request is not one of this case")
	} else if err != nil {
		return err
	}
	return nil
}

// AttachFiles marks more of a case's evidence files as sent in by one of its requests.
func AttachFiles(ctx context.Context, sqlDB *sql.DB, caseID string, requestID int64, fileIDs []int64) error {
	if err := Check(ctx, sqlDB, caseID, requestID); err != nil {
		return err
	}
	q := db.New(sqlDB)
	c, err := q.GetCaseByCaseID(ctx, caseID)
	if err != nil {
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
	caseRows, err := q.ListCaseRequestCases(ctx, caseID)
	if err != nil {
		return nil, err
	}
	referenceRows, err := q.ListCaseRequestReferences(ctx, caseID)
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
			ReceivedOn: r.ReceivedOn.Format(dateLayout), CitedReference: r.CitedReference.String,
			CitedInquiry: r.CitedInquiryNumber.String, Notes: r.Notes.String,
			RegisteredBy: r.RegisteredBy.String, CreatedAt: r.CreatedAt, Questions: []string{}, Files: []File{}, Cases: []CaseRef{}, Tasks: []string{}}
		if r.RequestingUnitID.Valid {
			req.RequestingUnitID = &r.RequestingUnitID.Int64
		}
		req.References = []Reference{}
		for _, x := range referenceRows {
			if x.CaseRequestID == r.ID {
				req.References = append(req.References, Reference{Label: x.Label, Number: x.Number})
			}
		}
		for _, a := range analysisRows {
			if a.CaseRequestID.Valid && a.CaseRequestID.Int64 == r.ID {
				req.Tasks = append(req.Tasks, a.AnalysisType)
			}
		}
		for _, cr := range caseRows {
			if cr.CaseRequestID == r.ID {
				req.Cases = append(req.Cases, CaseRef{CaseID: cr.CaseID, Modality: cr.Modality})
			}
		}
		if r.DocumentDate.Valid {
			req.DocumentDate = r.DocumentDate.Time.Format(dateLayout)
		}
		if r.DueOn.Valid {
			req.DueOn = r.DueOn.Time.Format(dateLayout)
			req.Overdue = open[r.ID] && today.After(r.DueOn.Time.AddDate(0, 0, 1))
		}
		if r.OccurrenceID.Valid {
			o := &Occurrence{ID: r.OccurrenceID.Int64, Agency: r.Agency.String, OccurrenceNumber: r.OccurrenceNumber.String,
				InquiryNumber: r.InquiryNumber.String}
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
