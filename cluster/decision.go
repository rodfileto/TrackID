package cluster

import (
	"context"
	"database/sql"
	"slices"

	"github.com/rodfileto/trackid/db"
)

// Role identifies who — or what — recorded one biometric_decisions row.
type Role string

const (
	RoleSystem        Role = "SYSTEM"
	RoleVerificator   Role = "VERIFICATOR"
	RoleReviewer      Role = "REVIEWER"
	RoleInconsistence Role = "INCONSISTENCE"
)

// Decision is the call one biometric_decisions row makes.
type Decision string

const (
	DecisionPositive     Decision = "POSITIVE"
	DecisionNegative     Decision = "NEGATIVE"
	DecisionInconclusive Decision = "INCONCLUSIVE"
)

// EdgeStatus is a pair's derived state, per DeriveEdgeStatus.
type EdgeStatus string

// Edge statuses derived from a pair's decision chain.
const (
	EdgeStatusPendingReview EdgeStatus = "PENDING_REVIEW"
	EdgeStatusConfirmed     EdgeStatus = "CONFIRMED"
	EdgeStatusDisputed      EdgeStatus = "DISPUTED"
	EdgeStatusRejected      EdgeStatus = "REJECTED"
)

// ChainEntry is the minimal shape of one biometric_decisions row DeriveEdgeStatus
// needs (role + decision). Exported so any caller -- an API handler recording an
// analyst decision, an import command seeding a SYSTEM one -- can derive or
// validate a pair's status without duplicating this package's sequencing rules.
type ChainEntry struct {
	Role     Role
	Decision Decision
	// Analysis is the case_analysis_id a human decision was made in; 0 for SYSTEM, or for a
	// human decision recorded before analyses existed.
	Analysis int64
	// AnalysisType is that analysis's type (FACIAL_REVIEW, FACIAL_COMPARISON), empty without one.
	AnalysisType string
	// Conclusion is a comparison's level for the pair (Conclusion), empty elsewhere.
	Conclusion Conclusion
}

// Counts reports whether e is part of round's chain: a SYSTEM decision, which counts in every
// round, or a human one made in that analysis. Round 0 is the SYSTEM decision with any human
// decisions recorded before analyses existed.
func (e ChainEntry) Counts(round int64) bool {
	return e.Role == RoleSystem || e.Analysis == round
}

// DecidingRound is the round whose outcome is the pair's status, and that status. A pair can be
// decided in several rounds -- the SYSTEM decision (round 0), a FACIAL_REVIEW, a
// FACIAL_COMPARISON -- each judged on its own by the rules of DeriveEdgeStatus. The pair keeps
// the outcome of its latest *settled* round (CONFIRMED or REJECTED): a later round that is still
// pending, disputed, or ended inconclusive (e.g. a comparison without the quality a review had)
// does not undo it. With no settled round, the latest round's status stands. ok is false only
// for an empty chain.
func DecidingRound(chain []ChainEntry) (round int64, status EdgeStatus, ok bool) {
	seen := map[int64]bool{}
	var rounds []int64
	for _, e := range chain {
		r := e.Analysis
		if e.Role == RoleSystem {
			r = 0
		}
		if !seen[r] {
			seen[r] = true
			rounds = append(rounds, r)
		}
	}
	slices.Sort(rounds)

	settled := false
	for _, r := range rounds {
		var roundChain []ChainEntry
		for _, e := range chain {
			if e.Counts(r) {
				roundChain = append(roundChain, e)
			}
		}
		s, _ := deriveRound(roundChain)
		isSettled := s == EdgeStatusConfirmed || s == EdgeStatusRejected
		if isSettled || !settled {
			round, status, ok = r, s, true
			settled = settled || isSettled
		}
	}
	return round, status, ok
}

// DeriveEdgeStatus is a pure function mapping a pair's full decision chain to its
// derived state. ok is false only for an empty chain (nothing decided yet). It is the status of
// the pair's deciding round (DecidingRound).
func DeriveEdgeStatus(chain []ChainEntry) (status EdgeStatus, ok bool) {
	_, status, ok = DecidingRound(chain)
	return status, ok
}

// Reviewed reports whether a pair's chain is CONFIRMED by people: its deciding round
// (DecidingRound) is CONFIRMED and was settled by a VERIFICATOR/REVIEWER agreement or by an
// INCONSISTENCE decision, not by a SYSTEM decision alone. A SYSTEM confirmation stays unreviewed
// even while a later human round is still pending, since that round does not undo it. Reviewed
// pairs build the REVIEWED cluster layer, the only one forensic views (a case, a person) read;
// every CONFIRMED pair builds the AUTOMATIC layer, for intelligence.
func Reviewed(chain []ChainEntry) bool {
	_, status, settledBy, ok := DecidingRoundBy(chain)
	return ok && status == EdgeStatusConfirmed && settledBy != RoleSystem
}

// DecidingRoundBy is DecidingRound that also names the role whose decision settled that round's
// status: INCONSISTENCE, REVIEWER (a VERIFICATOR/REVIEWER pair, agreeing or not), VERIFICATOR
// (alone, so still pending) or SYSTEM. A status settled by people is one whose settledBy is not
// SYSTEM.
func DecidingRoundBy(chain []ChainEntry) (round int64, status EdgeStatus, settledBy Role, ok bool) {
	round, status, ok = DecidingRound(chain)
	if !ok {
		return 0, "", "", false
	}
	var roundChain []ChainEntry
	for _, e := range chain {
		if e.Counts(round) {
			roundChain = append(roundChain, e)
		}
	}
	_, settledBy, _ = deriveRoundBy(roundChain)
	return round, status, settledBy, true
}

// deriveRound maps one round's chain -- the SYSTEM decision and the human decisions of one
// analysis -- to its state. The rules, in order:
//   - INCONSISTENCE, if present, is final: POSITIVE confirms, NEGATIVE rejects,
//     INCONCLUSIVE leaves the pair DISPUTED.
//   - VERIFICATOR and REVIEWER together settle the pair by agreement: matching
//     decisions confirm/reject (or, both INCONCLUSIVE, stay PENDING_REVIEW);
//     differing decisions are DISPUTED, and so are differing conclusion levels in a
//     comparison (the laudo states one level).
//   - VERIFICATOR alone is always PENDING_REVIEW.
//   - SYSTEM alone is CONFIRMED for a POSITIVE score, PENDING_REVIEW for
//     INCONCLUSIVE, or REJECTED for an explicit NEGATIVE.
func deriveRound(chain []ChainEntry) (status EdgeStatus, ok bool) {
	status, _, ok = deriveRoundBy(chain)
	return status, ok
}

// deriveRoundBy is deriveRound that also names the role whose decision settled the status:
// INCONSISTENCE, REVIEWER (for a VERIFICATOR/REVIEWER pair), VERIFICATOR (alone, always pending)
// or SYSTEM.
func deriveRoundBy(chain []ChainEntry) (status EdgeStatus, settledBy Role, ok bool) {
	var system, verificator, reviewer, inconsistence *Decision
	var verificatorLevel, reviewerLevel Conclusion
	for i := range chain {
		switch chain[i].Role {
		case RoleSystem:
			system = &chain[i].Decision
		case RoleVerificator:
			verificator = &chain[i].Decision
			verificatorLevel = chain[i].Conclusion
		case RoleReviewer:
			reviewer = &chain[i].Decision
			reviewerLevel = chain[i].Conclusion
		case RoleInconsistence:
			inconsistence = &chain[i].Decision
		}
	}

	if inconsistence != nil {
		return decisionOutcome(*inconsistence, EdgeStatusDisputed), RoleInconsistence, true
	}
	if verificator != nil && reviewer != nil {
		// In a comparison the two must reach the same level, not only the same decision.
		if *verificator == *reviewer && verificatorLevel == reviewerLevel {
			return decisionOutcome(*verificator, EdgeStatusPendingReview), RoleReviewer, true
		}
		return EdgeStatusDisputed, RoleReviewer, true
	}
	if verificator != nil {
		return EdgeStatusPendingReview, RoleVerificator, true
	}
	if system != nil {
		return decisionOutcome(*system, EdgeStatusPendingReview), RoleSystem, true
	}
	return "", "", false
}

func decisionOutcome(d Decision, inconclusive EdgeStatus) EdgeStatus {
	switch d {
	case DecisionPositive:
		return EdgeStatusConfirmed
	case DecisionNegative:
		return EdgeStatusRejected
	default:
		return inconclusive
	}
}

// confirmedPair is a pair of features whose decision chain has settled to
// CONFIRMED.
type confirmedPair struct {
	featureA string
	featureB string
	modality string // "FACIAL" | "FINGERPRINT"
	reviewed bool   // settled by people (Reviewed), not by a SYSTEM decision alone
}

// loadConfirmedPairs reads every biometric_decisions row, groups them into
// per-pair chains, and returns the pairs whose derived status is CONFIRMED,
// each marked with whether people confirmed it (Reviewed).
func loadConfirmedPairs(ctx context.Context, sqlDB *sql.DB) ([]confirmedPair, error) {
	rows, err := db.New(sqlDB).ListBiometricDecisions(ctx)
	if err != nil {
		return nil, err
	}

	type key struct{ a, b string }
	chains := map[key][]db.ListBiometricDecisionsRow{}
	var order []key
	for _, r := range rows {
		k := key{r.FeatureAID, r.FeatureBID}
		if _, ok := chains[k]; !ok {
			order = append(order, k)
		}
		chains[k] = append(chains[k], r)
	}

	var pairs []confirmedPair
	for _, k := range order {
		rowsForPair := chains[k]
		chain := make([]ChainEntry, len(rowsForPair))
		for i, r := range rowsForPair {
			chain[i] = ChainEntry{Role: Role(r.Role), Decision: Decision(r.Decision), Analysis: r.CaseAnalysisID.Int64,
				AnalysisType: r.AnalysisType.String, Conclusion: Conclusion(r.Conclusion.String)}
		}
		if _, status, ok := DecidingRound(chain); !ok || status != EdgeStatusConfirmed {
			continue
		}
		pairs = append(pairs, confirmedPair{
			featureA: k.a,
			featureB: k.b,
			modality: rowsForPair[0].Modality,
			reviewed: Reviewed(chain),
		})
	}
	return pairs, nil
}

// LinkBasis says what kind of human round settled a reviewed pair: a FACIAL_REVIEW (an
// investigative lead) or a FACIAL_COMPARISON (a laudo).
type LinkBasis string

const (
	LinkLead  LinkBasis = "LEAD"
	LinkLaudo LinkBasis = "LAUDO"
)

// The comparison types, as analyses names them (this package cannot import analyses). A facial
// comparison's laudo concludes on the graded scale; a fingerprint comparison's (confronto
// papiloscópico) positive, negative or inconclusive, a positive being an identification.
const (
	comparisonType            = "FACIAL_COMPARISON"
	fingerprintComparisonType = "FINGERPRINT_COMPARISON"
)

func isComparison(analysisType string) bool {
	return analysisType == comparisonType || analysisType == fingerprintComparisonType
}

// Link is how strong a reviewed pair is, from the round that settled it (DecidingRound): a lead,
// or a laudo at its conclusion level. Identified is every lead, and a laudo at or above the
// identification level; a laudo below it supports the link without identifying. LaterLaudo is a
// later comparison of the pair that did not settle it -- PENDING, DISPUTED or INCONCLUSIVE -- shown
// beside the lead it leaves standing.
type Link struct {
	Basis      LinkBasis  `json:"basis"`
	Conclusion Conclusion `json:"conclusion,omitempty"`
	Identified bool       `json:"identified"`
	LaterLaudo string     `json:"laterLaudo,omitempty"`
}

// ReviewedLink describes a reviewed pair (Reviewed) as Link; ok is false for a pair that is not
// reviewed. minimum is the identification level (IdentificationLevel).
func ReviewedLink(chain []ChainEntry, minimum Conclusion) (link Link, ok bool) {
	round, status, settledBy, ok := DecidingRoundBy(chain)
	if !ok || status != EdgeStatusConfirmed || settledBy == RoleSystem {
		return Link{}, false
	}
	link = Link{Basis: LinkLead, Identified: true}
	rounds := map[int64]string{}
	for _, e := range chain {
		if e.Role != RoleSystem {
			rounds[e.Analysis] = e.AnalysisType
		}
	}
	switch rounds[round] {
	case comparisonType:
		level := roundConclusion(chain, round)
		link = Link{Basis: LinkLaudo, Conclusion: level, Identified: level.AtLeast(minimum)}
	case fingerprintComparisonType:
		link = Link{Basis: LinkLaudo, Identified: true} // a confirmed positive
	}
	later := make([]int64, 0, len(rounds))
	for r := range rounds {
		later = append(later, r)
	}
	slices.Sort(later) // the latest unsettled comparison is the one shown
	for _, r := range later {
		if r <= round || !isComparison(rounds[r]) {
			continue
		}
		var roundChain []ChainEntry
		for _, e := range chain {
			if e.Role != RoleSystem && e.Analysis == r {
				roundChain = append(roundChain, e)
			}
		}
		s, by, _ := deriveRoundBy(roundChain)
		switch {
		case s == EdgeStatusDisputed:
			link.LaterLaudo = "DISPUTED"
		case s == EdgeStatusPendingReview && by == RoleReviewer:
			link.LaterLaudo = string(ConclusionInconclusive) // both examiners concluded inconclusive
		default:
			link.LaterLaudo = "PENDING"
		}
	}
	return link, true
}

// roundConclusion is the level a comparison round settled on: its INCONSISTENCE decision's, or
// else the agreed REVIEWER's.
func roundConclusion(chain []ChainEntry, round int64) Conclusion {
	var reviewer Conclusion
	for _, e := range chain {
		if e.Role == RoleSystem || e.Analysis != round {
			continue
		}
		switch e.Role {
		case RoleInconsistence:
			return e.Conclusion
		case RoleReviewer:
			reviewer = e.Conclusion
		}
	}
	return reviewer
}
