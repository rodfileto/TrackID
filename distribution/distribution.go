// Package distribution hands the work of a case's analyses (see the analyses package) out to
// examiners: one examiner per step -- the first examination (VERIFICATOR), the blind verification
// (REVIEWER), and the inconsistence when they disagree -- each distributed when the analysis
// reaches its stage, never to someone holding another step of the same analysis. Assignments are
// an append-only history (analysis_assignments, migrations 017-018): a step's current examiner is
// its latest row, and an open analysis whose current stage has none is pending.
//
// Distribute assigns up to a given number of pending items automatically, leaving the rest
// pending; Assign is an admin's manual override (reassign, or unassign back to pending).
package distribution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db"
)

// Assignment methods (analysis_assignments.method).
const (
	MethodAutomatic = "AUTOMATIC"
	MethodManual    = "MANUAL"
)

// Errors returned to callers.
var (
	ErrInvalidCount = errors.New("count must be at least 1")
	ErrUserNotFound = errors.New("user not found")
	ErrNotExaminer  = errors.New("user is not an active examiner of the analysis's modality")
	ErrClosed       = errors.New("the analysis is closed")
	ErrUnchanged    = errors.New("the analysis already has that assignment")
	ErrOtherStep    = errors.New("the user already holds another step of this analysis")
)

// Examiner is an active examiner and how many open analyses currently await their step.
type Examiner struct {
	UserID              int64  `json:"userId"`
	Username            string `json:"username"`
	ExaminesFingerprint bool   `json:"examinesFingerprint"`
	ExaminesFacial      bool   `json:"examinesFacial"`
	OpenWork            int    `json:"openWork"`
}

func (e Examiner) examines(m auth.Modality) bool {
	switch m {
	case auth.ModalityFingerprint:
		return e.ExaminesFingerprint
	case auth.ModalityFacial:
		return e.ExaminesFacial
	}
	return false
}

// PendingWork is an open analysis whose current stage has no examiner yet.
type PendingWork struct {
	ID       int64
	CaseID   string
	Sequence int16
	Type     analyses.Type
	Step     analyses.Step
	Exclude  []int64 // holders of the analysis's other steps
}

// Assignment is one step of one analysis handed to one examiner.
type Assignment struct {
	AnalysisID int64         `json:"analysisId"`
	CaseID     string        `json:"caseId"`
	Sequence   int16         `json:"sequence"`
	Type       analyses.Type `json:"type"`
	Step       analyses.Step `json:"step"`
	UserID     int64         `json:"userId"`
	Username   string        `json:"username"`
}

// plan is Distribute's pure core. It walks pending in order (oldest first) and gives each item to
// the eligible examiner -- one who examines its modality and holds no other step of the analysis
// -- with the fewest open items, the lowest user id breaking ties, counting each assignment as it
// goes so the load evens out. It stops after count assignments. An item no examiner can take is
// skipped (reported in noExaminer) and stays pending without using up the count.
func plan(pending []PendingWork, examiners []Examiner, count int) (assigned []Assignment, noExaminer []int64) {
	load := make([]int, len(examiners))
	for i, e := range examiners {
		load[i] = e.OpenWork
	}
	for _, a := range pending {
		if len(assigned) == count {
			break
		}
		best := -1
		for i, e := range examiners {
			if !e.examines(a.Type.Modality()) || slices.Contains(a.Exclude, e.UserID) {
				continue
			}
			if best < 0 || load[i] < load[best] || (load[i] == load[best] && e.UserID < examiners[best].UserID) {
				best = i
			}
		}
		if best < 0 {
			noExaminer = append(noExaminer, a.ID)
			continue
		}
		load[best]++
		assigned = append(assigned, Assignment{AnalysisID: a.ID, CaseID: a.CaseID, Sequence: a.Sequence, Type: a.Type,
			Step: a.Step, UserID: examiners[best].UserID, Username: examiners[best].Username})
	}
	return assigned, noExaminer
}

// Request is what Distribute is asked to do.
type Request struct {
	// Count is how many pending items to assign; the rest stay pending. At least 1.
	Count int `json:"count"`
	// Type limits the distribution to one analysis type; empty means every type.
	Type analyses.Type `json:"type,omitempty"`
	// DryRun computes the distribution without recording it.
	DryRun bool `json:"dryRun,omitempty"`
}

// Result is what Distribute did (or, on a dry run, would do).
type Result struct {
	Assigned []Assignment `json:"assigned"`
	// NoExaminer lists the analyses skipped because no eligible examiner was left for their step.
	NoExaminer []int64 `json:"noExaminer"`
	// StillPending is how many items (of the requested type) are pending afterwards.
	StillPending int  `json:"stillPending"`
	DryRun       bool `json:"dryRun"`
}

// Distribute assigns up to req.Count pending items, oldest first, each to the least-loaded eligible
// examiner (see plan), recording each as an AUTOMATIC assignment of its step. It holds a
// transaction-scoped lock, so concurrent distributions and manual assignments run one at a time.
func Distribute(ctx context.Context, sqlDB *sql.DB, req Request) (Result, error) {
	if req.Count < 1 {
		return Result{}, ErrInvalidCount
	}
	if req.Type != "" && !req.Type.Valid() {
		return Result{}, analyses.ErrInvalidType
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if err := q.LockCaseDistribution(ctx); err != nil {
		return Result{}, err
	}

	rows, err := q.ListPendingWork(ctx, sql.NullString{String: string(req.Type), Valid: req.Type != ""})
	if err != nil {
		return Result{}, err
	}
	pending := make([]PendingWork, len(rows))
	for i, r := range rows {
		pending[i] = PendingWork{ID: r.ID, CaseID: r.CaseID, Sequence: r.Sequence, Type: analyses.Type(r.AnalysisType),
			Step: analyses.Stage(r.Stage).ActiveStep()}
		for _, holder := range []sql.NullInt64{r.VerificatorID, r.ReviewerID} {
			if holder.Valid {
				pending[i].Exclude = append(pending[i].Exclude, holder.Int64)
			}
		}
	}
	examiners, err := listExaminers(ctx, q)
	if err != nil {
		return Result{}, err
	}

	assigned, noExaminer := plan(pending, examiners, req.Count)
	if !req.DryRun {
		for _, a := range assigned {
			if _, err := q.InsertAnalysisAssignment(ctx, db.InsertAnalysisAssignmentParams{
				CaseAnalysisID: a.AnalysisID,
				Step:           string(a.Step),
				UserID:         sql.NullInt64{Int64: a.UserID, Valid: true},
				Method:         MethodAutomatic,
			}); err != nil {
				return Result{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return Result{}, err
		}
	}
	if assigned == nil {
		assigned = []Assignment{}
	}
	if noExaminer == nil {
		noExaminer = []int64{}
	}
	return Result{Assigned: assigned, NoExaminer: noExaminer, StillPending: len(pending) - len(assigned), DryRun: req.DryRun}, nil
}

func listExaminers(ctx context.Context, q *db.Queries) ([]Examiner, error) {
	rows, err := q.ListExaminerLoads(ctx)
	if err != nil {
		return nil, err
	}
	examiners := make([]Examiner, len(rows))
	for i, r := range rows {
		examiners[i] = Examiner{UserID: r.ID, Username: r.Username, ExaminesFingerprint: r.ExaminesFingerprint,
			ExaminesFacial: r.ExaminesFacial, OpenWork: int(r.OpenWork)}
	}
	return examiners, nil
}

// Assign makes userID the examiner of an open analysis's current step (analyses.Stage.ActiveStep),
// or, with userID nil, unassigns that step back to pending; either way the step's draft decisions
// are discarded. The examiner must be active, examine the analysis's modality and hold no other
// step of it. by is who did it, recorded as assigned_by: an admin, or a user taking the review of a
// case they just created. Checking that by may do so is the caller's job.
func Assign(ctx context.Context, sqlDB *sql.DB, analysisID int64, userID *int64, by int64) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if err := q.LockCaseDistribution(ctx); err != nil {
		return err
	}

	target, err := analyses.GetTarget(ctx, q, analysisID)
	if err != nil {
		return err
	}
	if target.Status != analyses.StatusOpen {
		return ErrClosed
	}
	step := target.Stage.ActiveStep()
	if userID == nil {
		if target.ActiveExaminer() == 0 {
			return ErrUnchanged
		}
	} else {
		if target.ActiveExaminer() == *userID {
			return ErrUnchanged
		}
		if _, holds := target.StepOf(*userID); holds {
			return ErrOtherStep
		}
		user, err := auth.NewRepository(sqlDB).FindByID(ctx, *userID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		if !user.Examines(target.Type.Modality()) {
			return ErrNotExaminer
		}
	}

	var assignee sql.NullInt64
	if userID != nil {
		assignee = sql.NullInt64{Int64: *userID, Valid: true}
	}
	if err := q.DeleteDecisionDrafts(ctx, db.DeleteDecisionDraftsParams{CaseAnalysisID: analysisID, Step: string(step)}); err != nil {
		return err
	}
	if _, err := q.InsertAnalysisAssignment(ctx, db.InsertAnalysisAssignmentParams{
		CaseAnalysisID: analysisID,
		Step:           string(step),
		UserID:         assignee,
		Method:         MethodManual,
		AssignedBy:     sql.NullInt64{Int64: by, Valid: true},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// PendingCount is how many analyses of one type wait for an examiner at one stage.
type PendingCount struct {
	Type  analyses.Type  `json:"type"`
	Stage analyses.Stage `json:"stage"`
	Count int            `json:"count"`
}

// Status is the state of distribution: pending work per analysis type and stage, and each active
// examiner's load.
type Status struct {
	Pending   []PendingCount `json:"pending"`
	Examiners []Examiner     `json:"examiners"`
}

// GetStatus reports the pending work per analysis type and stage and each active examiner's load.
func GetStatus(ctx context.Context, sqlDB *sql.DB) (Status, error) {
	q := db.New(sqlDB)
	rows, err := q.CountPendingWork(ctx)
	if err != nil {
		return Status{}, err
	}
	status := Status{Pending: make([]PendingCount, len(rows))}
	for i, r := range rows {
		status.Pending[i] = PendingCount{Type: analyses.Type(r.AnalysisType), Stage: analyses.Stage(r.Stage), Count: int(r.Pending)}
	}
	if status.Examiners, err = listExaminers(ctx, q); err != nil {
		return Status{}, err
	}
	return status, nil
}

// HistoryEntry is one row of an analysis's assignment history. Username is empty for an
// unassignment; AssignedBy is empty for an AUTOMATIC row.
type HistoryEntry struct {
	Step       analyses.Step `json:"step"`
	Username   string        `json:"username,omitempty"`
	Method     string        `json:"method"`
	AssignedBy string        `json:"assignedBy,omitempty"`
	AssignedAt time.Time     `json:"assignedAt"`
}

// History returns an analysis's assignments, oldest first; the last one is current.
func History(ctx context.Context, sqlDB *sql.DB, analysisID int64) ([]HistoryEntry, error) {
	q := db.New(sqlDB)
	if _, err := analyses.GetTarget(ctx, q, analysisID); err != nil {
		return nil, err
	}
	rows, err := q.ListAnalysisAssignmentHistory(ctx, analysisID)
	if err != nil {
		return nil, err
	}
	history := make([]HistoryEntry, len(rows))
	for i, r := range rows {
		history[i] = HistoryEntry{Step: analyses.Step(r.Step), Username: r.Username.String, Method: r.Method, AssignedBy: r.AssignedByUsername.String, AssignedAt: r.AssignedAt}
	}
	return history, nil
}

// AssignedAnalysis is a step of an analysis currently assigned to a user, with its case. YourTurn
// is true when the analysis is open and at the stage this step works.
type AssignedAnalysis struct {
	CaseID      string         `json:"caseId"`
	CaseType    string         `json:"caseType"`
	Description string         `json:"description"`
	AnalysisID  int64          `json:"analysisId"`
	Sequence    int16          `json:"sequence"`
	Type        analyses.Type  `json:"type"`
	Status      string         `json:"status"`
	Stage       analyses.Stage `json:"stage"`
	Step        analyses.Step  `json:"step"`
	YourTurn    bool           `json:"yourTurn"`
	Method      string         `json:"method"`
	AssignedAt  time.Time      `json:"assignedAt"`
}

// AnalysesOf returns every step userID currently holds, open analyses first.
func AnalysesOf(ctx context.Context, sqlDB *sql.DB, userID int64) ([]AssignedAnalysis, error) {
	rows, err := db.New(sqlDB).ListAnalysesAssignedTo(ctx, sql.NullInt64{Int64: userID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list assigned analyses: %w", err)
	}
	out := make([]AssignedAnalysis, len(rows))
	for i, r := range rows {
		stage := analyses.Stage(r.Stage)
		out[i] = AssignedAnalysis{CaseID: r.CaseID, CaseType: r.CaseType, Description: r.Description, AnalysisID: r.ID,
			Sequence: r.Sequence, Type: analyses.Type(r.AnalysisType), Status: r.Status, Stage: stage, Step: analyses.Step(r.Step),
			YourTurn: r.Status == analyses.StatusOpen && stage.ActiveStep() == analyses.Step(r.Step),
			Method:   r.Method, AssignedAt: r.AssignedAt}
	}
	return out, nil
}
