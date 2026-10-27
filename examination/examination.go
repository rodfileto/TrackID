// Package examination is the work inside an analysis (see the analyses package): the pairs an
// examiner decides, their draft decisions, and submitting a step, which records the decisions in
// biometric_decisions and moves the analysis to its next stage.
//
// The pairs of a FACIAL_REVIEW are the matcher's candidates for the case's faces -- every SYSTEM
// decision touching one -- and those of a FACIAL_COMPARISON the pairs an admin listed. Either
// way only pairs whose other side is an enrolled person or another face of the same case are
// examined: pairs across cases are not distributed yet. At the INCONSISTENCE stage the pairs are
// only those the first two examiners decided differently.
//
// Verification and inconsistence are blind (analyses.BlindWork): while the analysis is at their
// stage, the REVIEWER and the INCONSISTENCE examiner see the two images of each pair and nothing
// else -- no earlier decision or who made it, no matcher score or decision, no names or case
// description. Drafts are only shown to the examiner whose step they are.
package examination

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/storage"
)

// Decisions an examiner can make (biometric_decisions.decision).
const (
	Positive     = "POSITIVE"
	Negative     = "NEGATIVE"
	Inconclusive = "INCONCLUSIVE"
)

// Errors returned to callers.
var (
	ErrInvalidDecision = errors.New("decision must be POSITIVE, NEGATIVE or INCONCLUSIVE")
	ErrNeedsConclusion = errors.New("a comparison decides each pair on the conclusion scale: conclusion is required and must match the decision")
	ErrNoConclusion    = errors.New("only a comparison records a conclusion level")
	ErrNotInScope      = errors.New("that pair is not one this step decides")
	ErrNothingToSubmit = errors.New("this stage has no decisions to submit")
)

// IncompleteError is Submit's refusal while pairs are still undecided.
type IncompleteError struct{ Missing int }

func (e IncompleteError) Error() string {
	return fmt.Sprintf("%d pair(s) still have no decision", e.Missing)
}

// Pair is two features, ordered as biometric_decisions keys them.
type Pair struct {
	FeatureA string `json:"featureA"`
	FeatureB string `json:"featureB"`
}

func ordered(a, b string) Pair {
	if a > b {
		a, b = b, a
	}
	return Pair{a, b}
}

// Decision is a decision on one pair: a draft, or one submitted in a step.
type Decision struct {
	Step       analyses.Step      `json:"step,omitempty"`
	Decision   string             `json:"decision"`
	Conclusion cluster.Conclusion `json:"conclusion,omitempty"` // a comparison's level (cluster.Conclusion)
	Notes      string             `json:"notes,omitempty"`
	Username   string             `json:"username,omitempty"`
}

// PairView is one pair as an examiner sees it. Questioned is a face of the case; Counterpart the
// enrolled person or other face it is compared with.
type PairView struct {
	Pair
	Questioned     cases.TraceLink `json:"questioned"`
	Counterpart    cases.TraceLink `json:"counterpart"`
	SystemDecision string          `json:"systemDecision,omitempty"` // the matcher's, when it proposed the pair
	Score          *float64        `json:"score,omitempty"`
	Draft          *Decision       `json:"draft,omitempty"`    // the viewer's own draft, when it is their step
	Decisions      []Decision      `json:"decisions"`          // submitted decisions the viewer may see
	Disputed       bool            `json:"disputed,omitempty"` // the first two examiners disagreed
}

// Worksheet is an analysis's pairs as one user sees them. YourStep is set when it is that user's
// turn to work (analyses.CanWork); Pairs are then the ones that step decides, otherwise the
// analysis's pairs as a whole.
type Worksheet struct {
	AnalysisID int64          `json:"analysisId"`
	CaseID     string         `json:"caseId"`
	Type       analyses.Type  `json:"type"`
	Status     string         `json:"status"`
	Stage      analyses.Stage `json:"stage"`
	YourStep   analyses.Step  `json:"yourStep,omitempty"`
	Blind      bool           `json:"blind"` // the viewer works it blind (analyses.BlindWork): images only
	Pairs      []PairView     `json:"pairs"`
	Drafted    int            `json:"drafted"`
}

type candidate struct {
	pair     Pair
	decision string
	score    *float64
}

// scope returns an analysis's pairs (see the package comment), with the matcher's decision and
// score when it proposed them, and the case's own face feature ids.
func scope(ctx context.Context, q *db.Queries, t analyses.Target) ([]candidate, map[string]bool, error) {
	traceIDs, err := q.ListCaseTraceIDs(ctx, t.BiometricCaseID)
	if err != nil {
		return nil, nil, err
	}
	own := map[string]bool{}
	ownIDs := make([]string, len(traceIDs))
	for i, id := range traceIDs {
		ownIDs[i] = graph.QuestionedFeatureID(id)
		own[ownIDs[i]] = true
	}
	inScope := func(p Pair) bool {
		ok := func(id string) bool {
			_, questioned := graph.ParseQuestionedFeatureID(id)
			return own[id] || !questioned
		}
		return (own[p.FeatureA] || own[p.FeatureB]) && ok(p.FeatureA) && ok(p.FeatureB)
	}

	system := map[Pair]candidate{}
	var systemOrder []Pair
	if len(ownIDs) > 0 {
		rows, err := q.ListSystemDecisionsForFeatures(ctx, ownIDs)
		if err != nil {
			return nil, nil, err
		}
		for _, r := range rows {
			p := Pair{r.FeatureAID, r.FeatureBID}
			c := candidate{pair: p, decision: r.Decision}
			if r.Confidence.Valid {
				v := r.Confidence.Float64
				c.score = &v
			}
			system[p] = c
			systemOrder = append(systemOrder, p)
		}
	}

	var out []candidate
	if t.Type == analyses.FacialComparison {
		rows, err := q.ListAnalysisPairs(ctx, t.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, r := range rows {
			p := Pair{r.FeatureAID, r.FeatureBID}
			if c, ok := system[p]; ok {
				out = append(out, c)
			} else if inScope(p) {
				out = append(out, candidate{pair: p})
			}
		}
	} else {
		for _, p := range systemOrder { // ranked by score, as a candidate list
			if inScope(p) {
				out = append(out, system[p])
			}
		}
	}
	return out, own, nil
}

// decisionsByPair groups an analysis's submitted decisions by pair and step.
func decisionsByPair(ctx context.Context, q *db.Queries, analysisID int64) (map[Pair]map[analyses.Step]Decision, error) {
	rows, err := q.ListAnalysisDecisions(ctx, sql.NullInt64{Int64: analysisID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := map[Pair]map[analyses.Step]Decision{}
	for _, r := range rows {
		p := Pair{r.FeatureAID, r.FeatureBID}
		if out[p] == nil {
			out[p] = map[analyses.Step]Decision{}
		}
		step := analyses.Step(r.Role)
		out[p][step] = Decision{Step: step, Decision: r.Decision, Conclusion: cluster.Conclusion(r.Conclusion.String), Notes: r.Notes.String,
			Username: r.Username.String}
	}
	return out, nil
}

// disputed reports whether the first two examiners decided p differently: another decision, or in
// a comparison another conclusion level.
func disputed(d map[analyses.Step]Decision) bool {
	v, okV := d[analyses.StepVerificator]
	r, okR := d[analyses.StepReviewer]
	return okV && okR && (v.Decision != r.Decision || v.Conclusion != r.Conclusion)
}

// stepScope narrows an analysis's pairs to those step decides: every pair, except at
// INCONSISTENCE, only the disputed ones.
func stepScope(all []candidate, step analyses.Step, decided map[Pair]map[analyses.Step]Decision) []candidate {
	if step != analyses.StepInconsistence {
		return all
	}
	var out []candidate
	for _, c := range all {
		if disputed(decided[c.pair]) {
			out = append(out, c)
		}
	}
	return out
}

// StepPairs returns the pairs the current stage of an open analysis decides: every pair, or at
// INCONSISTENCE the disputed ones. Distribution uses it to prefer examiners who have not decided
// them before (distribution.Distribute).
func StepPairs(ctx context.Context, q *db.Queries, t analyses.Target) ([]Pair, error) {
	all, _, err := scope(ctx, q, t)
	if err != nil {
		return nil, err
	}
	decided, err := decisionsByPair(ctx, q, t.ID)
	if err != nil {
		return nil, err
	}
	var out []Pair
	for _, c := range stepScope(all, t.Stage.ActiveStep(), decided) {
		out = append(out, c.pair)
	}
	return out, nil
}

// GetWorksheet returns an analysis's pairs as user sees them. Checking that user may open the
// case (distribution.CanOpen) is the caller's job.
func GetWorksheet(ctx context.Context, sqlDB *sql.DB, user auth.UserProfile, analysisID int64) (Worksheet, error) {
	q := db.New(sqlDB)
	t, err := analyses.GetTarget(ctx, q, analysisID)
	if err != nil {
		return Worksheet{}, err
	}
	all, own, err := scope(ctx, q, t)
	if err != nil {
		return Worksheet{}, err
	}
	decided, err := decisionsByPair(ctx, q, t.ID)
	if err != nil {
		return Worksheet{}, err
	}
	ws := Worksheet{AnalysisID: t.ID, CaseID: t.CaseID, Type: t.Type, Status: t.Status, Stage: t.Stage, Pairs: []PairView{}}
	pairs := all
	drafts := map[Pair]Decision{}
	if step, err := analyses.CanWork(user, t); err == nil && t.Stage != analyses.StageReport {
		ws.YourStep = step
		pairs = stepScope(all, step, decided)
		rows, err := q.ListDecisionDrafts(ctx, db.ListDecisionDraftsParams{CaseAnalysisID: t.ID, Step: string(step)})
		if err != nil {
			return Worksheet{}, err
		}
		for _, r := range rows {
			drafts[Pair{r.FeatureAID, r.FeatureBID}] = Decision{Step: step, Decision: r.Decision, Conclusion: cluster.Conclusion(r.Conclusion.String),
				Notes: r.Notes.String}
		}
	}
	// Blind work (analyses.BlindWork): the second and third examiners see the two images and
	// nothing else.
	blind := t.BlindTo(user.ID)
	ws.Blind = blind

	ids := make([]string, 0, 2*len(pairs))
	for _, c := range pairs {
		ids = append(ids, c.pair.FeatureA, c.pair.FeatureB)
	}
	described, err := cases.DescribeFeatures(ctx, q, ids)
	if err != nil {
		return Worksheet{}, err
	}
	for _, c := range pairs {
		questioned, counterpart := c.pair.FeatureA, c.pair.FeatureB
		if !own[questioned] {
			questioned, counterpart = counterpart, questioned
		}
		v := PairView{Pair: c.pair, Questioned: described[questioned], Counterpart: described[counterpart],
			SystemDecision: c.decision, Score: c.score, Decisions: []Decision{}, Disputed: disputed(decided[c.pair])}
		v.Questioned.FeatureID, v.Counterpart.FeatureID = questioned, counterpart // what PairImage takes
		if d, ok := drafts[c.pair]; ok {
			v.Draft = &d
			ws.Drafted++
		}
		if blind {
			v.Questioned, v.Counterpart = imageOnly(v.Questioned), imageOnly(v.Counterpart)
			v.SystemDecision, v.Score, v.Disputed = "", nil, false
			ws.Pairs = append(ws.Pairs, v)
			continue
		}
		for _, step := range analyses.Steps {
			if d, ok := decided[c.pair][step]; ok {
				v.Decisions = append(v.Decisions, d)
			}
		}
		ws.Pairs = append(ws.Pairs, v)
	}
	return ws, nil
}

// imageOnly keeps of a feature what a blind examiner needs to show it -- its kind, and the feature
// id PairImage serves its image by -- and nothing that names the person or describes the case.
func imageOnly(l cases.TraceLink) cases.TraceLink {
	return cases.TraceLink{Kind: l.Kind, FeatureID: l.FeatureID}
}

// SaveDraft records or replaces user's draft decision on one pair of the step they are working. In
// a comparison the decision comes with a conclusion level that maps onto it (cluster.Conclusion);
// in a review there is none.
func SaveDraft(ctx context.Context, sqlDB *sql.DB, user auth.UserProfile, analysisID int64, pair Pair, decision, notes string, conclusion cluster.Conclusion) error {
	if decision != Positive && decision != Negative && decision != Inconclusive {
		return ErrInvalidDecision
	}
	q := db.New(sqlDB)
	t, err := analyses.GetTarget(ctx, q, analysisID)
	if err != nil {
		return err
	}
	if t.Type == analyses.FacialComparison {
		if !conclusion.Valid() || string(conclusion.Decision()) != decision {
			return ErrNeedsConclusion
		}
	} else if conclusion != "" {
		return ErrNoConclusion
	}
	step, err := analyses.CanWork(user, t)
	if err != nil {
		return err
	}
	if t.Stage == analyses.StageReport {
		return ErrNothingToSubmit
	}
	all, _, err := scope(ctx, q, t)
	if err != nil {
		return err
	}
	decided, err := decisionsByPair(ctx, q, t.ID)
	if err != nil {
		return err
	}
	p := ordered(pair.FeatureA, pair.FeatureB)
	found := false
	for _, c := range stepScope(all, step, decided) {
		found = found || c.pair == p
	}
	if !found {
		return ErrNotInScope
	}
	return q.UpsertDecisionDraft(ctx, db.UpsertDecisionDraftParams{
		CaseAnalysisID: t.ID, Step: string(step), FeatureAID: p.FeatureA, FeatureBID: p.FeatureB,
		Decision: decision, Notes: sql.NullString{String: notes, Valid: notes != ""}, UserID: user.ID,
		Conclusion: sql.NullString{String: string(conclusion), Valid: conclusion != ""},
	})
}

// SubmitResult is what a submit did.
type SubmitResult struct {
	Decisions int            `json:"decisions"` // biometric_decisions rows written
	Stage     analyses.Stage `json:"stage"`     // the analysis's stage afterwards
}

// Submit records user's drafts for the step they are working as biometric_decisions -- every pair
// of the step must have one -- and moves the analysis on: from EXAMINATION to VERIFICATION (or
// straight to REPORT when there was nothing to decide), from VERIFICATION to INCONSISTENCE when the
// two examiners disagreed on a pair and to REPORT otherwise, from INCONSISTENCE to REPORT.
func Submit(ctx context.Context, sqlDB *sql.DB, user auth.UserProfile, analysisID int64) (SubmitResult, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return SubmitResult{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	t, err := analyses.GetTarget(ctx, q, analysisID)
	if err != nil {
		return SubmitResult{}, err
	}
	// One submit per case at a time, so a reassignment or a second tab cannot submit the same step twice.
	if _, err := q.LockBiometricCase(ctx, t.BiometricCaseID); err != nil {
		return SubmitResult{}, err
	}
	if t, err = analyses.GetTarget(ctx, q, analysisID); err != nil {
		return SubmitResult{}, err
	}
	step, err := analyses.CanWork(user, t)
	if err != nil {
		return SubmitResult{}, err
	}
	if t.Stage == analyses.StageReport {
		return SubmitResult{}, ErrNothingToSubmit
	}
	all, _, err := scope(ctx, q, t)
	if err != nil {
		return SubmitResult{}, err
	}
	decided, err := decisionsByPair(ctx, q, t.ID)
	if err != nil {
		return SubmitResult{}, err
	}
	pairs := stepScope(all, step, decided)
	rows, err := q.ListDecisionDrafts(ctx, db.ListDecisionDraftsParams{CaseAnalysisID: t.ID, Step: string(step)})
	if err != nil {
		return SubmitResult{}, err
	}
	drafts := map[Pair]db.ListDecisionDraftsRow{}
	for _, r := range rows {
		drafts[Pair{r.FeatureAID, r.FeatureBID}] = r
	}
	missing := 0
	for _, c := range pairs {
		if _, ok := drafts[c.pair]; !ok {
			missing++
		}
	}
	if missing > 0 {
		return SubmitResult{}, IncompleteError{Missing: missing}
	}

	for _, c := range pairs {
		d := drafts[c.pair]
		if _, err := q.InsertBiometricDecision(ctx, db.InsertBiometricDecisionParams{
			FeatureAID: c.pair.FeatureA, FeatureBID: c.pair.FeatureB, Modality: t.Type.DecisionModality(),
			Role: string(step), Decision: d.Decision, Username: sql.NullString{String: user.Username, Valid: true},
			Notes: d.Notes, CaseAnalysisID: sql.NullInt64{Int64: t.ID, Valid: true}, Conclusion: d.Conclusion,
		}); err != nil {
			return SubmitResult{}, err
		}
		if decided[c.pair] == nil {
			decided[c.pair] = map[analyses.Step]Decision{}
		}
		decided[c.pair][step] = Decision{Step: step, Decision: d.Decision, Conclusion: cluster.Conclusion(d.Conclusion.String)}
	}
	if err := q.DeleteDecisionDrafts(ctx, db.DeleteDecisionDraftsParams{CaseAnalysisID: t.ID, Step: string(step)}); err != nil {
		return SubmitResult{}, err
	}

	next := analyses.StageReport
	switch t.Stage {
	case analyses.StageExamination:
		if len(pairs) > 0 {
			next = analyses.StageVerification
		}
	case analyses.StageVerification:
		for _, c := range pairs {
			if disputed(decided[c.pair]) {
				next = analyses.StageInconsistence
				break
			}
		}
	}
	if err := q.SetCaseAnalysisStage(ctx, db.SetCaseAnalysisStageParams{ID: t.ID, Stage: string(next)}); err != nil {
		return SubmitResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SubmitResult{}, err
	}
	return SubmitResult{Decisions: len(pairs), Stage: next}, nil
}

// Record is one pair of an analysis with the matcher's proposal and every decision submitted on
// it, by step: what its report states.
type Record struct {
	Pair
	SystemDecision string                     `json:"systemDecision,omitempty"`
	Score          *float64                   `json:"score,omitempty"`
	Decisions      map[analyses.Step]Decision `json:"decisions"`
	Disputed       bool                       `json:"disputed"`
}

// Records returns every pair of an analysis (its scope, as GetWorksheet's) with its decisions, in
// the analysis's order, and the case's own face feature ids.
func Records(ctx context.Context, q *db.Queries, t analyses.Target) ([]Record, map[string]bool, error) {
	all, own, err := scope(ctx, q, t)
	if err != nil {
		return nil, nil, err
	}
	decided, err := decisionsByPair(ctx, q, t.ID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Record, len(all))
	for i, c := range all {
		d := decided[c.pair]
		if d == nil {
			d = map[analyses.Step]Decision{}
		}
		out[i] = Record{Pair: c.pair, SystemDecision: c.decision, Score: c.score, Decisions: d, Disputed: disputed(d)}
	}
	return out, own, nil
}

// FeatureImage returns the image of any feature, a questioned one cut to its box: PairImage
// without the analysis's scope, for the report the server builds.
func FeatureImage(ctx context.Context, sqlDB *sql.DB, store *storage.Client, featureID string) (Image, error) {
	return featureImage(ctx, sqlDB, store, featureID)
}
