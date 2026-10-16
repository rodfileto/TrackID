// Package analyses holds the rounds of examination a biometric case goes through (case_analyses,
// migration 017). Each analysis has a type, which fixes its modality, its methodology and the
// report it leads to; an examiner (see the distribution package); the human decisions made in it
// (biometric_decisions.case_analysis_id); and one report, a DRAFT until its examiner issues it.
//
// For faces, following FISWG: every FACIAL case gets a FACIAL_REVIEW automatically (one-to-many:
// the facial recognition system's candidates, giving an investigative lead or a negative
// result), and an admin opens a FACIAL_COMPARISON (one-to-one forensic examination) on the
// specific pairs that need one. Fingerprint analyses are not modelled yet.
package analyses

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// Type is an analysis type (case_analyses.analysis_type).
type Type string

const (
	FacialReview     Type = "FACIAL_REVIEW"
	FacialComparison Type = "FACIAL_COMPARISON"
)

// Valid reports whether t is an analysis type case_analyses accepts.
func (t Type) Valid() bool {
	return t == FacialReview || t == FacialComparison
}

// Modality is the biometric an analysis of type t examines.
func (t Type) Modality() auth.Modality {
	return auth.ModalityFacial
}

// ReportType is the report an analysis of type t leads to (analysis_reports.report_type): for a
// review, a report of investigative leads; for a comparison, a forensic report (laudo).
func (t Type) ReportType() string {
	return string(t) + "_REPORT"
}

// Analysis statuses (case_analyses.status) and report statuses (analysis_reports.status).
const (
	StatusOpen   = "OPEN"
	StatusClosed = "CLOSED"

	ReportDraft  = "DRAFT"
	ReportIssued = "ISSUED"
)

// Errors returned to callers.
var (
	ErrInvalidType      = errors.New("invalid analysis type")
	ErrCaseNotFound     = errors.New("case not found")
	ErrAnalysisNotFound = errors.New("analysis not found")
	ErrWrongModality    = errors.New("the analysis type does not match the case's modality")
	ErrNeedsPairs       = errors.New("a comparison needs at least one pair")
	ErrUnexpectedPairs  = errors.New("only a comparison lists pairs")
	ErrInvalidPair      = errors.New("each pair needs two different features, at least one a face of this case")
	ErrCannotDecide     = errors.New("only the analysis's examiner, examining its modality, can work on it while it is open")
	ErrNotDraft         = errors.New("the report is already issued")
)

// Pair is two features, as biometric_decisions keys them (graph feature ids).
type Pair struct {
	FeatureA string `json:"featureA"`
	FeatureB string `json:"featureB"`
}

// Report is an analysis's report. Number is "<year>.<sequence>" once issued, empty while a draft.
type Report struct {
	Type     string     `json:"type"`
	Status   string     `json:"status"`
	Number   string     `json:"number,omitempty"`
	IssuedBy string     `json:"issuedBy,omitempty"`
	IssuedAt *time.Time `json:"issuedAt,omitempty"`
}

// Analysis is one analysis of a case.
type Analysis struct {
	ID          int64      `json:"id"`
	Sequence    int16      `json:"sequence"`
	Type        Type       `json:"type"`
	Status      string     `json:"status"`
	RequestedBy string     `json:"requestedBy,omitempty"` // empty: opened automatically with the case
	Examiner    string     `json:"examiner,omitempty"`    // empty: pending
	ExaminerID  int64      `json:"examinerId,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ClosedAt    *time.Time `json:"closedAt,omitempty"`
	Pairs       []Pair     `json:"pairs"` // a comparison's pairs; empty for a review
	Report      Report     `json:"report"`
}

func reportNumber(year, number int32) string {
	return fmt.Sprintf("%04d.%04d", year, number)
}

// OpenAutomatic opens the analyses every case of its modality starts with -- a FACIAL_REVIEW for
// a FACIAL case -- unless the case already has one, so re-ingesting a case does not open another.
// It runs inside the transaction that creates or ingests the case and returns the review's id
// (0 when nothing was opened).
func OpenAutomatic(ctx context.Context, q *db.Queries, biometricCaseID int64, modality string) (int64, error) {
	if modality != string(auth.ModalityFacial) {
		return 0, nil
	}
	n, err := q.CountCaseAnalysesOfType(ctx, db.CountCaseAnalysesOfTypeParams{BiometricCaseID: biometricCaseID, AnalysisType: string(FacialReview)})
	if err != nil || n > 0 {
		return 0, err
	}
	return insert(ctx, q, biometricCaseID, FacialReview, sql.NullInt64{})
}

func insert(ctx context.Context, q *db.Queries, biometricCaseID int64, t Type, requestedBy sql.NullInt64) (int64, error) {
	if _, err := q.LockBiometricCase(ctx, biometricCaseID); err != nil {
		return 0, err
	}
	row, err := q.InsertCaseAnalysis(ctx, db.InsertCaseAnalysisParams{BiometricCaseID: biometricCaseID, AnalysisType: string(t), RequestedBy: requestedBy})
	if err != nil {
		return 0, err
	}
	if err := q.InsertAnalysisReport(ctx, db.InsertAnalysisReportParams{CaseAnalysisID: row.ID, ReportType: t.ReportType()}); err != nil {
		return 0, err
	}
	return row.ID, nil
}

// Open is an admin opening an analysis on a case: a FACIAL_COMPARISON on the given pairs, or
// another FACIAL_REVIEW round (no pairs). Each pair needs at least one face (trace) of this case.
// requestedBy is the admin; checking that they are one is the caller's job. The analysis starts
// pending, for distribution.
func Open(ctx context.Context, sqlDB *sql.DB, caseID string, t Type, pairs []Pair, requestedBy int64) (int64, error) {
	if !t.Valid() {
		return 0, ErrInvalidType
	}
	if t == FacialComparison && len(pairs) == 0 {
		return 0, ErrNeedsPairs
	}
	if t != FacialComparison && len(pairs) > 0 {
		return 0, ErrUnexpectedPairs
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	q := db.New(tx)

	c, err := q.GetCaseByCaseID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrCaseNotFound
	}
	if err != nil {
		return 0, err
	}
	if c.Modality != string(t.Modality()) {
		return 0, ErrWrongModality
	}
	traceIDs, err := q.ListCaseTraceIDs(ctx, c.ID)
	if err != nil {
		return 0, err
	}
	ofCase := map[string]bool{}
	for _, id := range traceIDs {
		ofCase[graph.QuestionedFeatureID(id)] = true
	}
	ordered := make([]Pair, 0, len(pairs))
	seen := map[Pair]bool{}
	for _, p := range pairs {
		a, b := p.FeatureA, p.FeatureB
		if a > b {
			a, b = b, a
		}
		if a == b || !validFeatureID(a) || !validFeatureID(b) || !(ofCase[a] || ofCase[b]) {
			return 0, fmt.Errorf("%w: %s / %s", ErrInvalidPair, p.FeatureA, p.FeatureB)
		}
		if !seen[Pair{a, b}] {
			seen[Pair{a, b}] = true
			ordered = append(ordered, Pair{a, b})
		}
	}

	id, err := insert(ctx, q, c.ID, t, sql.NullInt64{Int64: requestedBy, Valid: true})
	if err != nil {
		return 0, err
	}
	for _, p := range ordered {
		if err := q.InsertCaseAnalysisPair(ctx, db.InsertCaseAnalysisPairParams{CaseAnalysisID: id, FeatureAID: p.FeatureA, FeatureBID: p.FeatureB}); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// validFeatureID accepts the two graph feature id forms: a QUESTIONED "TRACE:<id>#feature" or a
// KNOWN bare identity_file id.
func validFeatureID(id string) bool {
	if _, ok := graph.ParseQuestionedFeatureID(id); ok {
		return true
	}
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0
}

// List returns a case's analyses in order, each with its examiner, pairs and report.
func List(ctx context.Context, sqlDB *sql.DB, caseID string) ([]Analysis, error) {
	q := db.New(sqlDB)
	if _, err := q.GetCaseByCaseID(ctx, caseID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCaseNotFound
	} else if err != nil {
		return nil, err
	}
	rows, err := q.ListCaseAnalyses(ctx, caseID)
	if err != nil {
		return nil, err
	}
	pairRows, err := q.ListCaseAnalysisPairs(ctx, caseID)
	if err != nil {
		return nil, err
	}
	pairs := map[int64][]Pair{}
	for _, p := range pairRows {
		pairs[p.CaseAnalysisID] = append(pairs[p.CaseAnalysisID], Pair{FeatureA: p.FeatureAID, FeatureB: p.FeatureBID})
	}
	out := make([]Analysis, len(rows))
	for i, r := range rows {
		a := Analysis{ID: r.ID, Sequence: r.Sequence, Type: Type(r.AnalysisType), Status: r.Status,
			RequestedBy: r.RequestedBy.String, Examiner: r.Examiner.String, ExaminerID: r.ExaminerID.Int64,
			CreatedAt: r.CreatedAt, Pairs: pairs[r.ID],
			Report: Report{Type: r.ReportType.String, Status: r.ReportStatus.String, IssuedBy: r.IssuedBy.String}}
		if a.Pairs == nil {
			a.Pairs = []Pair{}
		}
		if r.ClosedAt.Valid {
			a.ClosedAt = &r.ClosedAt.Time
		}
		if r.ReportNumber.Valid {
			a.Report.Number = reportNumber(r.ReportYear.Int32, r.ReportNumber.Int32)
		}
		if r.IssuedAt.Valid {
			a.Report.IssuedAt = &r.IssuedAt.Time
		}
		out[i] = a
	}
	return out, nil
}

// Target is an analysis with its case and current examiner, as GetTarget reads it.
type Target struct {
	ID         int64
	Type       Type
	Status     string
	CaseID     string
	ExaminerID int64 // 0 when pending
}

// GetTarget reads one analysis, or ErrAnalysisNotFound.
func GetTarget(ctx context.Context, q *db.Queries, analysisID int64) (Target, error) {
	r, err := q.GetCaseAnalysis(ctx, analysisID)
	if errors.Is(err, sql.ErrNoRows) {
		return Target{}, ErrAnalysisNotFound
	}
	if err != nil {
		return Target{}, err
	}
	return Target{ID: r.ID, Type: Type(r.AnalysisType), Status: r.Status, CaseID: r.CaseID, ExaminerID: r.UserID.Int64}, nil
}

// CanDecide reports whether user may work on an analysis -- record its human decisions, issue its
// report: only its current examiner, while they are active and examine its modality, and only
// while it is open. Being an admin lets nobody decide on an analysis they do not hold.
func CanDecide(user auth.UserProfile, t Target) error {
	if t.ExaminerID != 0 && t.ExaminerID == user.ID && user.Examines(t.Type.Modality()) && t.Status == StatusOpen {
		return nil
	}
	return ErrCannotDecide
}

// IssueReport issues an analysis's report: it numbers it "<year>.<sequence>" per report type and
// year, records who issued it, and closes the analysis. Only CanDecide's user may.
func IssueReport(ctx context.Context, sqlDB *sql.DB, analysisID int64, user auth.UserProfile, now time.Time) (Report, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	t, err := GetTarget(ctx, q, analysisID)
	if err != nil {
		return Report{}, err
	}
	if err := CanDecide(user, t); err != nil {
		return Report{}, err
	}
	if err := q.LockReportNumbering(ctx); err != nil {
		return Report{}, err
	}
	year := int32(now.Year())
	max, err := q.MaxReportNumber(ctx, db.MaxReportNumberParams{ReportType: t.Type.ReportType(), ReportYear: sql.NullInt32{Int32: year, Valid: true}})
	if err != nil {
		return Report{}, err
	}
	row, err := q.IssueAnalysisReport(ctx, db.IssueAnalysisReportParams{
		CaseAnalysisID: analysisID,
		ReportYear:     sql.NullInt32{Int32: year, Valid: true},
		ReportNumber:   sql.NullInt32{Int32: max + 1, Valid: true},
		IssuedBy:       sql.NullInt64{Int64: user.ID, Valid: true},
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, ErrNotDraft
	}
	if err != nil {
		return Report{}, err
	}
	if err := q.CloseCaseAnalysis(ctx, analysisID); err != nil {
		return Report{}, err
	}
	if err := tx.Commit(); err != nil {
		return Report{}, err
	}
	return Report{Type: row.ReportType, Status: ReportIssued, Number: reportNumber(row.ReportYear.Int32, row.ReportNumber.Int32),
		IssuedBy: user.Username, IssuedAt: &row.IssuedAt.Time}, nil
}
