package cluster

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rodfileto/trackid/db"
)

// Conclusion is a laudo's conclusion on a pair (biometric_decisions.conclusion), on a graded scale
// after FISWG's: what a FACIAL_COMPARISON decides, where a FACIAL_REVIEW decides only POSITIVE,
// NEGATIVE or INCONCLUSIVE. Every level maps onto a Decision (Decision), so the rounds logic
// (DecidingRound) reads the decision alone.
type Conclusion string

const (
	ConclusionIdentification   Conclusion = "IDENTIFICATION"
	ConclusionStrongSupport    Conclusion = "STRONG_SUPPORT"
	ConclusionModerateSupport  Conclusion = "MODERATE_SUPPORT"
	ConclusionLimitedSupport   Conclusion = "LIMITED_SUPPORT"
	ConclusionInconclusive     Conclusion = "INCONCLUSIVE"
	ConclusionLimitedExclusion Conclusion = "LIMITED_EXCLUSION"
	ConclusionExclusion        Conclusion = "EXCLUSION"
)

// Conclusions lists the scale from the strongest support to the strongest exclusion.
var Conclusions = []Conclusion{
	ConclusionIdentification, ConclusionStrongSupport, ConclusionModerateSupport, ConclusionLimitedSupport,
	ConclusionInconclusive, ConclusionLimitedExclusion, ConclusionExclusion,
}

// DefaultIdentificationLevel is the lowest conclusion that counts as an identification while
// identification_levels holds no version.
const DefaultIdentificationLevel = ConclusionStrongSupport

// Valid reports whether c is a level of the scale.
func (c Conclusion) Valid() bool {
	return c.rank() >= 0
}

// rank is c's position from the strongest exclusion (0) up; -1 for no level.
func (c Conclusion) rank() int {
	for i, l := range Conclusions {
		if l == c {
			return len(Conclusions) - 1 - i
		}
	}
	return -1
}

// Decision is the decision c maps onto: the support levels POSITIVE, INCONCLUSIVE itself, the
// exclusions NEGATIVE (the database's conclusion_decision).
func (c Conclusion) Decision() Decision {
	switch c {
	case ConclusionIdentification, ConclusionStrongSupport, ConclusionModerateSupport, ConclusionLimitedSupport:
		return DecisionPositive
	case ConclusionLimitedExclusion, ConclusionExclusion:
		return DecisionNegative
	}
	return DecisionInconclusive
}

// AtLeast reports whether c is a support level at or above minimum.
func (c Conclusion) AtLeast(minimum Conclusion) bool {
	return c.Decision() == DecisionPositive && c.rank() >= minimum.rank()
}

// IdentificationLevel is the current lowest conclusion that counts as an identification: the
// latest identification_levels version, or DefaultIdentificationLevel.
func IdentificationLevel(ctx context.Context, q *db.Queries) (Conclusion, error) {
	v, err := q.CurrentIdentificationLevel(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultIdentificationLevel, nil
	}
	if err != nil {
		return "", err
	}
	return Conclusion(v), nil
}
