package cluster

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rodfileto/trackid/db"
)

// Conclusion is a laudo's conclusion on a pair (biometric_decisions.conclusion): what a
// FACIAL_COMPARISON decides, where a FACIAL_REVIEW decides only POSITIVE, NEGATIVE or
// INCONCLUSIVE. The scale is the five opinion categories of OSAC 2022-S-0001 (Standard Guide for
// Image Comparison Opinions), which FISWG's ACE-V guideline for one-to-one examinations (v1.0,
// §5.3.2) and its quality management draft (v1.0, §14.7.1) point to: the strength of support the
// observations give to a common source against different sources. OSAC subdivides the support
// categories only where research shows examiners apply finer ones reliably, and rules out
// "identification" and "exclusion" in the scale's wording. Every level maps onto a Decision
// (Decision), so the rounds logic (DecidingRound) reads the decision alone.
type Conclusion string

const (
	ConclusionStrongSupportCommon    Conclusion = "STRONG_SUPPORT_COMMON"
	ConclusionSupportCommon          Conclusion = "SUPPORT_COMMON"
	ConclusionInconclusive           Conclusion = "INCONCLUSIVE"
	ConclusionSupportDifferent       Conclusion = "SUPPORT_DIFFERENT"
	ConclusionStrongSupportDifferent Conclusion = "STRONG_SUPPORT_DIFFERENT"
)

// Conclusions lists the scale from the strongest support for a common source to the strongest
// support for different sources.
var Conclusions = []Conclusion{
	ConclusionStrongSupportCommon, ConclusionSupportCommon, ConclusionInconclusive,
	ConclusionSupportDifferent, ConclusionStrongSupportDifferent,
}

// DefaultIdentificationLevel is the lowest conclusion whose link counts as identified in TrackID
// (the person profile, the badges) while identification_levels holds no version.
const DefaultIdentificationLevel = ConclusionStrongSupportCommon

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

// Decision is the decision c maps onto: support for a common source POSITIVE, INCONCLUSIVE
// itself, support for different sources NEGATIVE (the database's conclusion_decision).
func (c Conclusion) Decision() Decision {
	switch c {
	case ConclusionStrongSupportCommon, ConclusionSupportCommon:
		return DecisionPositive
	case ConclusionSupportDifferent, ConclusionStrongSupportDifferent:
		return DecisionNegative
	}
	return DecisionInconclusive
}

// AtLeast reports whether c supports a common source at or above minimum.
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
