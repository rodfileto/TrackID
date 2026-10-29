// Package analyses holds the rounds of examination a biometric case goes through (case_analyses,
// db/migrations/006_analyses.sql). Each analysis has a type, which fixes its modality, its methodology and the
// report it leads to; an examiner (see the distribution package); the human decisions made in it
// (biometric_decisions.case_analysis_id); and one report, a DRAFT until its examiner issues it.
//
// Analyses are the tasks requests ask for (docs/request-routing-plan.md): registering a request
// routes each of its cases to a task, chosen by the coordinator, and opens an analysis of that type
// answering the request. Nothing opens automatically. For faces, following FISWG, a FACIAL_REVIEW
// is one-to-many (the facial recognition system's candidates, giving an investigative lead or a
// negative result) and a FACIAL_COMPARISON one-to-one (a forensic examination, a laudo). For
// fingerprints, a FINGERPRINT_COMPARISON (confronto papiloscópico) is the one-to-one examination,
// concluding positive, negative or inconclusive. A comparison's pairs are listed by the
// coordinator, at opening or afterwards (AddPairs). The laboratory and scene examinations are
// reserved types, not offered yet.
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
	"github.com/sqlc-dev/pqtype"
)

// Type is an analysis type (case_analyses.analysis_type).
type Type string

const (
	FacialReview          Type = "FACIAL_REVIEW"
	FacialComparison      Type = "FACIAL_COMPARISON"
	FingerprintComparison Type = "FINGERPRINT_COMPARISON"
	// Reserved, not offered yet: "coming soon" in the screens.
	FingerprintLabExamination Type = "FINGERPRINT_LAB_EXAMINATION"
	CrimeSceneExamination     Type = "CRIME_SCENE_EXAMINATION"
)

// Types lists every analysis type, offered or not.
var Types = []Type{FacialReview, FacialComparison, FingerprintComparison, FingerprintLabExamination, CrimeSceneExamination}

// Valid reports whether t is an analysis type case_analyses accepts.
func (t Type) Valid() bool {
	for _, v := range Types {
		if t == v {
			return true
		}
	}
	return false
}

// Offered reports whether an analysis of type t can be opened: the laboratory and scene
// examinations are reserved for later.
func (t Type) Offered() bool {
	return t == FacialReview || t == FacialComparison || t == FingerprintComparison
}

// Comparison reports whether t is a one-to-one examination whose pairs the coordinator lists, and
// which leads to a laudo.
func (t Type) Comparison() bool {
	return t == FacialComparison || t == FingerprintComparison
}

// GradedConclusion reports whether the decisions of an analysis of type t carry a level on the
// laudo's conclusion scale (cluster.Conclusion): a facial comparison's do; a fingerprint
// comparison concludes positive, negative or inconclusive.
func (t Type) GradedConclusion() bool {
	return t == FacialComparison
}

// Modality is the biometric an analysis of type t examines.
func (t Type) Modality() auth.Modality {
	if t == FacialReview || t == FacialComparison {
		return auth.ModalityFacial
	}
	return auth.ModalityFingerprint
}

// DecisionModality is how biometric_decisions.modality names the modality of type t: as
// biometric_cases.modality does.
func (t Type) DecisionModality() string {
	return string(t.Modality())
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

// Stage is where an open analysis is (case_analyses.stage): each stage but the
// last is worked by one examiner, in one Step.
type Stage string

const (
	StageExamination   Stage = "EXAMINATION"
	StageVerification  Stage = "VERIFICATION"
	StageInconsistence Stage = "INCONSISTENCE"
	StageReport        Stage = "REPORT"
)

// Step is one examiner's part in an analysis, named after the biometric_decisions role their
// decisions take: the first examiner (VERIFICATOR, FISWG's practitioner), the second, blind one
// (REVIEWER, FISWG's verification) and the third, on disagreement (INCONSISTENCE).
type Step string

const (
	StepVerificator   Step = "VERIFICATOR"
	StepReviewer      Step = "REVIEWER"
	StepInconsistence Step = "INCONSISTENCE"
)

// Steps lists every step in order.
var Steps = []Step{StepVerificator, StepReviewer, StepInconsistence}

// ActiveStep is the step whose examiner works stage s: the one deciding in it, or, at REPORT, the
// first examiner, who issues the report.
func (s Stage) ActiveStep() Step {
	switch s {
	case StageVerification:
		return StepReviewer
	case StageInconsistence:
		return StepInconsistence
	}
	return StepVerificator
}

// BlindWork reports whether an open analysis at stage is worked blind by its step's examiner:
// the second examiner at VERIFICATION and the third at INCONSISTENCE decide from the two images
// alone -- no earlier decision, no one's name, no matcher score or decision, nothing of the case --
// so neither the first examiner's conclusion nor anyone's authority can lead them.
func BlindWork(stage Stage, step Step) bool {
	return step != StepVerificator && stage.ActiveStep() == step
}

// BlindTo reports whether userID works the analysis blind right now (BlindWork).
func (t Target) BlindTo(userID int64) bool {
	step, ok := t.StepOf(userID)
	return ok && t.Status == StatusOpen && BlindWork(t.Stage, step)
}

// Errors returned to callers.
var (
	ErrInvalidType      = errors.New("invalid analysis type")
	ErrCaseNotFound     = errors.New("case not found")
	ErrAnalysisNotFound = errors.New("analysis not found")
	ErrWrongModality    = errors.New("the analysis type does not match the case's modality")
	ErrNotOffered       = errors.New("this kind of examination is not available yet")
	ErrUnexpectedPairs  = errors.New("only a comparison lists pairs")
	ErrNotComparison    = errors.New("pairs are added to a comparison only")
	ErrPairsClosed      = errors.New("pairs can be added only while the comparison is open at its examination stage")
	ErrInvalidPair      = errors.New("each pair needs two different features: a trace of this case, and another trace of it or an enrolled person")
	ErrCannotDecide     = errors.New("only the examiner of the analysis's current stage, examining its modality, can work on it while it is open")
	ErrNotReportStage   = errors.New("the report can only be issued once every pair is settled")
	ErrNotDraft         = errors.New("the report is already issued")
	ErrUnknownRequest   = errors.New("the request is not one of this case")
	ErrBadComplement    = errors.New("a complement names an issued report of this case, of the same type")
)

// Pair is two features, as biometric_decisions keys them (graph feature ids).
type Pair struct {
	FeatureA string `json:"featureA"`
	FeatureB string `json:"featureB"`
}

// Report is an analysis's report. Number is "<year>.<sequence>" once issued, empty while a draft.
// Complements names the report a laudo complementar complements, by number.
type Report struct {
	ID          int64      `json:"id"`
	Complements string     `json:"complements,omitempty"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	Number      string     `json:"number,omitempty"`
	IssuedBy    string     `json:"issuedBy,omitempty"`
	IssuedAt    *time.Time `json:"issuedAt,omitempty"`
}

// Examiner is who holds one step of an analysis.
type Examiner struct {
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}

// Analysis is one analysis of a case.
type Analysis struct {
	ID          int64             `json:"id"`
	Sequence    int16             `json:"sequence"`
	Type        Type              `json:"type"`
	Status      string            `json:"status"`
	Stage       Stage             `json:"stage"`
	RequestedBy string            `json:"requestedBy,omitempty"` // empty: opened with no user (an import's request, or before tasks were routed)
	Examiners   map[Step]Examiner `json:"examiners"`             // current examiner per step; a missing step is unassigned
	CreatedAt   time.Time         `json:"createdAt"`
	ClosedAt    *time.Time        `json:"closedAt,omitempty"`
	Pairs       []Pair            `json:"pairs"`               // a comparison's pairs; empty for a review
	RequestID   int64             `json:"requestId,omitempty"` // the request it answers (requests package)
	Report      Report            `json:"report"`
}

func reportNumber(year, number int32) string {
	return fmt.Sprintf("%04d.%04d", year, number)
}

// Options are what else an analysis can be opened with.
type Options struct {
	// RequestID is the request it answers (case_requests); 0 is the case's latest one, if any.
	RequestID int64
	// Complements is the case analysis whose issued report this one's report complements (a laudo
	// complementar); 0 for none. It must be of the same type.
	Complements int64
}

func insert(ctx context.Context, q *db.Queries, biometricCaseID int64, t Type, requestedBy, requestID sql.NullInt64) (int64, error) {
	if _, err := q.LockBiometricCase(ctx, biometricCaseID); err != nil {
		return 0, err
	}
	row, err := q.InsertCaseAnalysis(ctx, db.InsertCaseAnalysisParams{BiometricCaseID: biometricCaseID, AnalysisType: string(t), RequestedBy: requestedBy,
		CaseRequestID: requestID})
	if err != nil {
		return 0, err
	}
	if err := q.InsertAnalysisReport(ctx, db.InsertAnalysisReportParams{CaseAnalysisID: row.ID, ReportType: t.ReportType()}); err != nil {
		return 0, err
	}
	return row.ID, nil
}

// OpenIn opens an analysis of type t on the case biometricCaseID (of modality caseModality) inside
// the caller's transaction, answering requestID: routing a request to its task
// (requests.Register, requests.Create). requestedBy is the coordinator, 0 for none (an import).
// A comparison opens without pairs; AddPairs adds them.
func OpenIn(ctx context.Context, q *db.Queries, biometricCaseID int64, caseModality string, t Type, requestID, requestedBy int64) (int64, error) {
	if !t.Valid() {
		return 0, ErrInvalidType
	}
	if !t.Offered() {
		return 0, ErrNotOffered
	}
	if caseModality != string(t.Modality()) {
		return 0, ErrWrongModality
	}
	return insert(ctx, q, biometricCaseID, t, sql.NullInt64{Int64: requestedBy, Valid: requestedBy != 0},
		sql.NullInt64{Int64: requestID, Valid: requestID != 0})
}

// Open is a coordinator opening an analysis on a case: a comparison, on the given pairs or none
// yet (AddPairs adds them), or a review (no pairs). Each pair needs at least one trace of this
// case. requestedBy is the coordinator; checking that they are one is the caller's job. The
// analysis starts pending, for distribution.
func Open(ctx context.Context, sqlDB *sql.DB, caseID string, t Type, pairs []Pair, requestedBy int64) (int64, error) {
	return OpenWith(ctx, sqlDB, caseID, t, pairs, requestedBy, Options{})
}

// OpenWith is Open with Options: the request the analysis answers, and the report it complements.
func OpenWith(ctx context.Context, sqlDB *sql.DB, caseID string, t Type, pairs []Pair, requestedBy int64, opts Options) (int64, error) {
	if !t.Valid() {
		return 0, ErrInvalidType
	}
	if !t.Offered() {
		return 0, ErrNotOffered
	}
	if !t.Comparison() && len(pairs) > 0 {
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
	ordered, err := validPairs(ctx, q, c.ID, pairs)
	if err != nil {
		return 0, err
	}

	requestID := sql.NullInt64{}
	if opts.RequestID != 0 {
		if _, err := q.GetCaseRequestOfCase(ctx, db.GetCaseRequestOfCaseParams{CaseRequestID: opts.RequestID, BiometricCaseID: c.ID}); errors.Is(err, sql.ErrNoRows) {
			return 0, ErrUnknownRequest
		} else if err != nil {
			return 0, err
		}
		requestID = sql.NullInt64{Int64: opts.RequestID, Valid: true}
	} else if latest, err := q.LatestCaseRequest(ctx, c.ID); err == nil {
		requestID = sql.NullInt64{Int64: latest, Valid: true}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var complements int64
	if opts.Complements != 0 {
		r, err := q.GetAnalysisReport(ctx, opts.Complements)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrBadComplement
		}
		if err != nil {
			return 0, err
		}
		prior, err := GetTarget(ctx, q, opts.Complements)
		if err != nil || prior.BiometricCaseID != c.ID || prior.Type != t || r.Status != ReportIssued {
			return 0, ErrBadComplement
		}
		complements = r.ID
	}

	id, err := insert(ctx, q, c.ID, t, sql.NullInt64{Int64: requestedBy, Valid: true}, requestID)
	if err != nil {
		return 0, err
	}
	if complements != 0 {
		r, err := q.GetAnalysisReport(ctx, id)
		if err != nil {
			return 0, err
		}
		if err := q.SetReportComplements(ctx, db.SetReportComplementsParams{ID: r.ID, ComplementsReportID: sql.NullInt64{Int64: complements, Valid: true}}); err != nil {
			return 0, err
		}
	}
	for _, p := range ordered {
		if err := q.InsertCaseAnalysisPair(ctx, db.InsertCaseAnalysisPairParams{CaseAnalysisID: id, FeatureAID: p.FeatureA, FeatureBID: p.FeatureB}); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// validPairs orders pairs as biometric_decisions keys them, drops repeats, and checks each: a trace
// of the case against another trace of it or an enrolled person, so an examiner holding the
// analysis can see both sides (pairs across cases are not distributed yet).
func validPairs(ctx context.Context, q *db.Queries, biometricCaseID int64, pairs []Pair) ([]Pair, error) {
	traceIDs, err := q.ListCaseTraceIDs(ctx, biometricCaseID)
	if err != nil {
		return nil, err
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
		if a == b || !(ofCase[a] || ofCase[b]) || !(ofCase[a] || isKnown(a)) || !(ofCase[b] || isKnown(b)) {
			return nil, fmt.Errorf("%w: %s / %s", ErrInvalidPair, p.FeatureA, p.FeatureB)
		}
		if !seen[Pair{a, b}] {
			seen[Pair{a, b}] = true
			ordered = append(ordered, Pair{a, b})
		}
	}
	return ordered, nil
}

// AddPairs adds pairs to a comparison of caseID that is open at its examination stage: a
// comparison a request opened empty, or one that needs more. Pairs it already has are kept once.
// Checking the caller is a coordinator is the caller's job.
func AddPairs(ctx context.Context, sqlDB *sql.DB, caseID string, analysisID int64, pairs []Pair) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	t, err := GetTarget(ctx, q, analysisID)
	if errors.Is(err, ErrAnalysisNotFound) || (err == nil && t.CaseID != caseID) {
		return ErrAnalysisNotFound
	}
	if err != nil {
		return err
	}
	if !t.Type.Comparison() {
		return ErrNotComparison
	}
	if t.Status != StatusOpen || t.Stage != StageExamination {
		return ErrPairsClosed
	}
	if _, err := q.LockBiometricCase(ctx, t.BiometricCaseID); err != nil {
		return err
	}
	ordered, err := validPairs(ctx, q, t.BiometricCaseID, pairs)
	if err != nil {
		return err
	}
	existing, err := q.ListAnalysisPairs(ctx, analysisID)
	if err != nil {
		return err
	}
	has := map[Pair]bool{}
	for _, r := range existing {
		has[Pair{r.FeatureAID, r.FeatureBID}] = true
	}
	for _, p := range ordered {
		if has[p] {
			continue
		}
		if err := q.InsertCaseAnalysisPair(ctx, db.InsertCaseAnalysisPairParams{CaseAnalysisID: analysisID, FeatureAID: p.FeatureA, FeatureBID: p.FeatureB}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// isKnown reports whether id is a KNOWN feature id: a bare identity_file id.
func isKnown(id string) bool {
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
		a := Analysis{ID: r.ID, Sequence: r.Sequence, Type: Type(r.AnalysisType), Status: r.Status, Stage: Stage(r.Stage),
			RequestedBy: r.RequestedBy.String, Examiners: map[Step]Examiner{}, CreatedAt: r.CreatedAt, Pairs: pairs[r.ID],
			RequestID: r.CaseRequestID.Int64,
			Report:    Report{ID: r.ReportID.Int64, Type: r.ReportType.String, Status: r.ReportStatus.String, IssuedBy: r.IssuedBy.String}}
		if a.Pairs == nil {
			a.Pairs = []Pair{}
		}
		for step, id := range map[Step]sql.NullInt64{StepVerificator: r.VerificatorID, StepReviewer: r.ReviewerID, StepInconsistence: r.InconsistenceID} {
			if id.Valid {
				name := map[Step]sql.NullString{StepVerificator: r.Verificator, StepReviewer: r.Reviewer, StepInconsistence: r.Inconsistence}[step]
				a.Examiners[step] = Examiner{UserID: id.Int64, Username: name.String}
			}
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
	numbers := map[int64]string{}
	for _, a := range out {
		numbers[a.Report.ID] = a.Report.Number
	}
	for i, r := range rows {
		if r.ComplementsReportID.Valid {
			out[i].Report.Complements = numbers[r.ComplementsReportID.Int64]
		}
	}
	return out, nil
}

// Target is an analysis with its case and its current examiner per step, as GetTarget reads it.
type Target struct {
	ID              int64
	Type            Type
	Status          string
	Stage           Stage
	BiometricCaseID int64
	CaseID          string
	Examiners       map[Step]int64 // user id per step; a missing step is unassigned
}

// ActiveExaminer is the user id holding the step of the analysis's current stage, 0 if none.
func (t Target) ActiveExaminer() int64 {
	return t.Examiners[t.Stage.ActiveStep()]
}

// StepOf returns the step user holds in the analysis, if any (one person never holds two).
func (t Target) StepOf(userID int64) (Step, bool) {
	for _, step := range Steps {
		if id, ok := t.Examiners[step]; ok && id == userID {
			return step, true
		}
	}
	return "", false
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
	t := Target{ID: r.ID, Type: Type(r.AnalysisType), Status: r.Status, Stage: Stage(r.Stage), BiometricCaseID: r.BiometricCaseID,
		CaseID: r.CaseID, Examiners: map[Step]int64{}}
	for step, id := range map[Step]sql.NullInt64{StepVerificator: r.VerificatorID, StepReviewer: r.ReviewerID, StepInconsistence: r.InconsistenceID} {
		if id.Valid {
			t.Examiners[step] = id.Int64
		}
	}
	return t, nil
}

// CanWork reports whether user may work on an analysis now -- decide on its pairs, submit, or, at
// REPORT, issue its report -- and in which step: only the examiner holding the step of its current
// stage, while they are active and examine its modality, and only while it is open. Being an
// admin lets nobody work on an analysis step they do not hold.
func CanWork(user auth.UserProfile, t Target) (Step, error) {
	step := t.Stage.ActiveStep()
	if t.Status == StatusOpen && t.ActiveExaminer() == user.ID && user.ID != 0 && user.Examines(t.Type.Modality()) {
		return step, nil
	}
	return "", ErrCannotDecide
}

// Documents produces an issued report's documents inside IssueReport's transaction, once the
// report has its number: the frozen data, and the .docx and PDF as case_files rows. The reports
// package provides it.
type Documents func(ctx context.Context, q *db.Queries, t Target, r Report) (Issued, error)

// Issued is what Documents produced.
type Issued struct {
	Snapshot   []byte // JSON
	DocxFileID int64
	PDFFileID  int64
}

// IssueReport issues an analysis's report once every pair is settled (stage REPORT): it numbers it
// "<year>.<sequence>" per report type and year, records who issued it, has docs produce its
// documents, and closes the analysis -- all or nothing. Only its first examiner may (CanWork at
// REPORT).
func IssueReport(ctx context.Context, sqlDB *sql.DB, analysisID int64, user auth.UserProfile, now time.Time, docs Documents) (Report, error) {
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
	if t.Status == StatusOpen && t.Stage != StageReport {
		return Report{}, ErrNotReportStage
	}
	if _, err := CanWork(user, t); err != nil {
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
	current, err := q.GetAnalysisReport(ctx, analysisID)
	if err != nil {
		return Report{}, err
	}
	if current.Status != ReportDraft {
		return Report{}, ErrNotDraft
	}
	issuedAt := now
	report := Report{ID: current.ID, Type: current.ReportType, Status: ReportIssued, Number: reportNumber(year, max+1),
		IssuedBy: user.Username, IssuedAt: &issuedAt}
	out, err := docs(ctx, q, t, report)
	if err != nil {
		return Report{}, err
	}
	if err := q.SetReportDocuments(ctx, db.SetReportDocumentsParams{ID: current.ID,
		CaseFileID: sql.NullInt64{Int64: out.PDFFileID, Valid: out.PDFFileID != 0},
		DocxFileID: sql.NullInt64{Int64: out.DocxFileID, Valid: out.DocxFileID != 0},
		Snapshot:   pqtype.NullRawMessage{RawMessage: out.Snapshot, Valid: true}}); err != nil {
		return Report{}, err
	}
	if _, err := q.IssueAnalysisReport(ctx, db.IssueAnalysisReportParams{
		CaseAnalysisID: analysisID,
		ReportYear:     sql.NullInt32{Int32: year, Valid: true},
		ReportNumber:   sql.NullInt32{Int32: max + 1, Valid: true},
		IssuedBy:       sql.NullInt64{Int64: user.ID, Valid: true},
		IssuedAt:       sql.NullTime{Time: now, Valid: true},
	}); errors.Is(err, sql.ErrNoRows) {
		return Report{}, ErrNotDraft
	} else if err != nil {
		return Report{}, err
	}
	if err := q.CloseCaseAnalysis(ctx, analysisID); err != nil {
		return Report{}, err
	}
	if err := tx.Commit(); err != nil {
		return Report{}, err
	}
	return report, nil
}
