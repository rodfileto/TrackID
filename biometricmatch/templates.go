package biometricmatch

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rodfileto/trackid/db"
)

// TemplateMatcher scores a probe template against candidate templates, one
// score per candidate in order. fingerprint.Client implements it over the
// sourceafis-sidecar service; the interface keeps this package free of any
// particular matcher.
type TemplateMatcher interface {
	Match(ctx context.Context, probe []byte, candidates [][]byte) ([]float64, error)
}

// templateBatch caps how many candidate templates go to the matcher in one
// call.
const templateBatch = 256

// RunTemplates is Run for biometric_templates: opaque matcher templates
// rather than vectors, so there is no ANN index to narrow the search.
// Every unmatched template of templateType is scored by matcher against every
// other template of that type, and each pair scoring at least threshold
// (the matcher's own scale -- e.g. SourceAFIS's 40, not a cosine distance)
// gets a SYSTEM POSITIVE biometric_decisions row, unless a SYSTEM decision
// already exists for it. The unmatched templates are then marked matched.
//
// The unmatched template is the probe. Two unmatched templates are scored
// once, with the earlier one (by id) as probe. The matcher is called before
// the write transaction opens, as Run does with its ANN reads.
func RunTemplates(ctx context.Context, sqlDB *sql.DB, matcher TemplateMatcher, templateType string, threshold float64, systemSource string) (Stats, error) {
	if sqlDB == nil {
		return Stats{}, fmt.Errorf("database is not configured")
	}
	if matcher == nil {
		return Stats{}, fmt.Errorf("template matcher is not configured")
	}
	if systemSource == "" {
		return Stats{}, fmt.Errorf("systemSource is required")
	}

	candidates, templateIDs, err := computeTemplateCandidates(ctx, sqlDB, matcher, templateType, threshold)
	if err != nil {
		return Stats{}, err
	}
	if err := recordDecisions(ctx, sqlDB, candidates, systemSource, "", templateIDs, (*db.Queries).MarkBiometricTemplateMatched); err != nil {
		return Stats{}, err
	}
	return Stats{Embeddings: len(templateIDs), Decisions: len(candidates)}, nil
}

func computeTemplateCandidates(ctx context.Context, sqlDB *sql.DB, matcher TemplateMatcher, templateType string, threshold float64) ([]candidate, []int64, error) {
	all, err := db.New(sqlDB).ListBiometricTemplates(ctx, templateType)
	if err != nil {
		return nil, nil, err
	}

	var unmatchedIDs []int64
	for _, t := range all {
		if t.Unmatched {
			unmatchedIDs = append(unmatchedIDs, t.ID)
		}
	}
	if len(unmatchedIDs) == 0 {
		return nil, nil, nil
	}

	features, err := loadFeatureInfos(ctx, sqlDB)
	if err != nil {
		return nil, nil, err
	}
	existing, err := existingSystemPairs(ctx, sqlDB)
	if err != nil {
		return nil, nil, err
	}

	var candidates []candidate
	seen := map[string]bool{}

	// all is ordered by id, so for probe at index i every unmatched template
	// before it has already been scored against it as a probe.
	for i, probe := range all {
		if !probe.Unmatched {
			continue
		}
		probeInfo, ok := features[probe.BiometricfeatureID]
		if !ok {
			continue
		}

		var gallery []db.ListBiometricTemplatesRow
		for j, c := range all {
			if j == i || (j < i && c.Unmatched) {
				continue
			}
			gallery = append(gallery, c)
		}

		for start := 0; start < len(gallery); start += templateBatch {
			batch := gallery[start:min(start+templateBatch, len(gallery))]
			templates := make([][]byte, len(batch))
			for k, c := range batch {
				templates[k] = c.Template
			}
			scores, err := matcher.Match(ctx, probe.Template, templates)
			if err != nil {
				return nil, nil, fmt.Errorf("biometricmatch: match template %d: %w", probe.ID, err)
			}
			if len(scores) != len(batch) {
				return nil, nil, fmt.Errorf("biometricmatch: match template %d: %d scores for %d candidates", probe.ID, len(scores), len(batch))
			}

			for k, score := range scores {
				if score < threshold {
					continue
				}
				candidateInfo, ok := features[batch[k].BiometricfeatureID]
				if !ok {
					continue
				}
				a, b := probeInfo.graphID, candidateInfo.graphID
				if a == b {
					continue
				}
				if a > b {
					a, b = b, a
				}
				key := a + "|" + b
				if existing[key] || seen[key] {
					continue
				}
				seen[key] = true
				candidates = append(candidates, candidate{
					featureAID: a,
					featureBID: b,
					modality:   probeInfo.modality,
					confidence: score,
					decision:   "POSITIVE",
					threshold:  threshold,
				})
			}
		}
	}

	return candidates, unmatchedIDs, nil
}
