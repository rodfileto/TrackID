package cases

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// CaseCluster is one biometric cluster (see person.Cluster) that at least one
// of a case's codified traces resolved into. Traces of the same case that
// share a cluster are merged into a single entry (LocalTraceIDs) rather than
// repeating the same cluster once per trace.
type CaseCluster struct {
	ClusterID     int64       `json:"clusterId"`
	MemberCount   int         `json:"memberCount"`
	LocalTraceIDs []int64     `json:"localTraceIds"`
	Links         []TraceLink `json:"links"`
}

// ThumbnailBox is a face's bounding box within a TraceLink's
// ThumbnailFileID image, in that image's own pixel coordinates -- present
// only when the thumbnail file isn't already a tight crop of just the face
// (see person.ThumbnailBox, the identical shape that package uses).
type ThumbnailBox struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
}

// TraceLink is one other biometric sample sharing a case cluster -- another
// case's trace, or an enrolled person's KNOWN feature. DecisionRole/
// Automatic/DecidedBy describe the CONFIRMED biometric_decisions chain
// directly linking one of the cluster's local traces to this one, when there
// is a direct pair to point to (the most human-reviewed one, if the cluster's
// local traces disagree on which pair to show); a member reached only
// transitively through other cluster members leaves those fields blank
// rather than overclaiming a link that was never itself decided.
//
// ThumbnailFileID/ThumbnailBox (QUESTIONED) and IdentityFileID/ContentType
// (KNOWN) tell a caller how to render this link as a thumbnail -- same
// source priority as person.ClusterMember/CaseFaceSearchResult: for a
// QUESTIONED member, ThumbnailFileID names the trace's own face_crop file
// when it has one (ThumbnailBox nil, nothing left to crop) or the evidence
// file plus the trace's box otherwise; it's 0 when neither is available
// (e.g. the evidence file was later excluded). ThumbnailFileID is scoped to
// CaseID, not the case this link's cluster was requested for -- download it
// from that other case.
type TraceLink struct {
	Kind string `json:"kind"` // "KNOWN" | "QUESTIONED"

	PersonID       string `json:"personId,omitempty"`
	Name           string `json:"name,omitempty"`
	IdentityFileID int64  `json:"identityFileId,omitempty"`
	ContentType    string `json:"contentType,omitempty"`

	CaseID          string        `json:"caseId,omitempty"`
	CaseType        string        `json:"caseType,omitempty"`
	Description     string        `json:"description,omitempty"`
	TraceID         int64         `json:"traceId,omitempty"`
	ThumbnailFileID int64         `json:"thumbnailFileId,omitempty"`
	ThumbnailBox    *ThumbnailBox `json:"thumbnailBox,omitempty"`

	DecisionRole string   `json:"decisionRole,omitempty"` // SYSTEM | VERIFICATOR | REVIEWER | INCONSISTENCE
	Automatic    bool     `json:"automatic"`
	DecidedBy    string   `json:"decidedBy,omitempty"`
	Confidence   *float64 `json:"confidence,omitempty"`
}

// ListCaseClusters resolves the distinct biometric clusters a case's codified
// traces have resolved into, and the other cases/persons each cluster is
// CONFIRMED-linked to -- pointing out whether that link is automatic (a
// SYSTEM decision alone) or was human-reviewed (a VERIFICATOR/REVIEWER/
// INCONSISTENCE decision in the chain). A trace with no biometricfeature or
// no match yet contributes no entry -- only clusters actually reached are
// returned, in the order their first trace appears in ListCaseCodifications.
func ListCaseClusters(ctx context.Context, sqlDB *sql.DB, caseID string) ([]CaseCluster, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("cases: nil db")
	}

	codifications, err := ListCaseCodifications(ctx, sqlDB, caseID)
	if err != nil {
		return nil, err
	}

	var traceIDs []int64
	seenTrace := map[int64]bool{}
	for _, c := range codifications {
		if seenTrace[c.TraceID] {
			continue
		}
		seenTrace[c.TraceID] = true
		traceIDs = append(traceIDs, c.TraceID)
	}
	if len(traceIDs) == 0 {
		return nil, nil
	}

	featureIDs := make([]string, len(traceIDs))
	traceOfFeature := make(map[string]int64, len(traceIDs))
	for i, id := range traceIDs {
		fid := graph.QuestionedFeatureID(id)
		featureIDs[i] = fid
		traceOfFeature[fid] = id
	}

	q := db.New(sqlDB)

	membershipRows, err := q.ListClusterMembershipsForFeatureIDs(ctx, featureIDs)
	if err != nil {
		return nil, err
	}
	clusterOfTrace := make(map[int64]int64, len(membershipRows))
	var clusterIDs []int64
	seenCluster := map[int64]bool{}
	for _, r := range membershipRows {
		traceID, ok := traceOfFeature[r.FeatureID]
		if !ok {
			continue
		}
		clusterOfTrace[traceID] = r.ClusterID
		if !seenCluster[r.ClusterID] {
			seenCluster[r.ClusterID] = true
			clusterIDs = append(clusterIDs, r.ClusterID)
		}
	}

	// Group this case's own traces by the cluster they share, in the order
	// each cluster is first reached -- the merge the caller asked for, so a
	// cluster holding several of this case's faces is reported once.
	var clusterOrder []int64
	localTraces := map[int64][]int64{}
	for _, traceID := range traceIDs {
		clusterID, ok := clusterOfTrace[traceID]
		if !ok {
			continue
		}
		if _, seen := localTraces[clusterID]; !seen {
			clusterOrder = append(clusterOrder, clusterID)
		}
		localTraces[clusterID] = append(localTraces[clusterID], traceID)
	}
	if len(clusterOrder) == 0 {
		return nil, nil
	}

	membersByCluster := map[int64][]string{}
	var identityFileIDs, otherTraceIDs []int64
	seenIdentityFile := map[int64]bool{}
	seenOtherTrace := map[int64]bool{}
	memberRows, err := q.ListClusterMembersByClusterIDs(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}
	for _, m := range memberRows {
		membersByCluster[m.ClusterID] = append(membersByCluster[m.ClusterID], m.FeatureID)
		if tid, ok := graph.ParseQuestionedFeatureID(m.FeatureID); ok {
			if !seenOtherTrace[tid] {
				seenOtherTrace[tid] = true
				otherTraceIDs = append(otherTraceIDs, tid)
			}
			continue
		}
		if id, err := strconv.ParseInt(m.FeatureID, 10, 64); err == nil {
			if !seenIdentityFile[id] {
				seenIdentityFile[id] = true
				identityFileIDs = append(identityFileIDs, id)
			}
		}
	}

	knownByFile := map[int64]db.ListKnownClusterMembersRow{}
	if len(identityFileIDs) > 0 {
		rows, err := q.ListKnownClusterMembers(ctx, identityFileIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			knownByFile[r.IdentityFileID] = r
		}
	}

	questionedByTrace := map[int64]db.ListQuestionedClusterMembersRow{}
	if len(otherTraceIDs) > 0 {
		rows, err := q.ListQuestionedClusterMembers(ctx, otherTraceIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			questionedByTrace[r.CaseTraceID] = r
		}
	}

	decisionRows, err := q.ListBiometricDecisionsForFeatures(ctx, featureIDs)
	if err != nil {
		return nil, err
	}
	// chainsByOwn[ownFeature][counterpartFeature] is that pair's decision
	// chain, from our own trace's point of view.
	chainsByOwn := map[string]map[string][]db.ListBiometricDecisionsForFeaturesRow{}
	for _, r := range decisionRows {
		if chainsByOwn[r.FeatureID] == nil {
			chainsByOwn[r.FeatureID] = map[string][]db.ListBiometricDecisionsForFeaturesRow{}
		}
		chainsByOwn[r.FeatureID][r.CounterpartID] = append(chainsByOwn[r.FeatureID][r.CounterpartID], r)
	}

	out := make([]CaseCluster, 0, len(clusterOrder))
	for _, clusterID := range clusterOrder {
		localTraceIDs := localTraces[clusterID]
		localFeatures := make(map[string]bool, len(localTraceIDs))
		for _, tid := range localTraceIDs {
			localFeatures[graph.QuestionedFeatureID(tid)] = true
		}
		members := membersByCluster[clusterID]

		var links []TraceLink
		for _, m := range members {
			if localFeatures[m] {
				continue // one of this case's own faces, listed via LocalTraceIDs instead
			}

			var link TraceLink
			if tid, ok := graph.ParseQuestionedFeatureID(m); ok {
				d, ok := questionedByTrace[tid]
				if !ok {
					// The trace's evidence was excluded/deleted since clustering.
					continue
				}
				link = TraceLink{Kind: "QUESTIONED", CaseID: d.CaseID, CaseType: d.CaseType, Description: d.Description, TraceID: tid}
				switch {
				case d.TraceCropFileID.Valid:
					link.ThumbnailFileID = d.TraceCropFileID.Int64
				case d.EvidenceFileID.Valid && d.BoxX1.Valid:
					link.ThumbnailFileID = d.EvidenceFileID.Int64
					link.ThumbnailBox = &ThumbnailBox{X1: d.BoxX1.Float64, Y1: d.BoxY1.Float64, X2: d.BoxX2.Float64, Y2: d.BoxY2.Float64}
				}
			} else {
				id, err := strconv.ParseInt(m, 10, 64)
				if err != nil {
					continue
				}
				d, ok := knownByFile[id]
				if !ok {
					continue
				}
				link = TraceLink{Kind: "KNOWN", PersonID: d.PersonID, Name: d.Name, IdentityFileID: d.IdentityFileID, ContentType: d.ContentType.String}
			}

			if role, decidedBy, automatic, confidence, ok := bestDecision(localTraceIDs, m, chainsByOwn); ok {
				link.DecisionRole = role
				link.Automatic = automatic
				link.DecidedBy = decidedBy
				link.Confidence = confidence
			}

			links = append(links, link)
		}

		out = append(out, CaseCluster{
			ClusterID:     clusterID,
			MemberCount:   len(members),
			LocalTraceIDs: localTraceIDs,
			Links:         links,
		})
	}
	return out, nil
}

// bestDecision picks the most informative CONFIRMED decision linking member
// to any of localTraceIDs -- a human-reviewed one (VERIFICATOR/REVIEWER/
// INCONSISTENCE) over an automatic SYSTEM one, since several of this case's
// own traces can each carry their own direct pair with member and disagree on
// how thoroughly it was reviewed. ok is false when none of them has a direct
// CONFIRMED chain with member (it's only reachable transitively).
func bestDecision(localTraceIDs []int64, member string, chainsByOwn map[string]map[string][]db.ListBiometricDecisionsForFeaturesRow) (role, decidedBy string, automatic bool, confidence *float64, ok bool) {
	for _, tid := range localTraceIDs {
		own := graph.QuestionedFeatureID(tid)
		chain := chainsByOwn[own][member]
		if len(chain) == 0 {
			continue
		}
		entries := make([]cluster.ChainEntry, len(chain))
		for i, r := range chain {
			entries[i] = cluster.ChainEntry{Role: cluster.Role(r.Role), Decision: cluster.Decision(r.Decision)}
		}
		status, derived := cluster.DeriveEdgeStatus(entries)
		if !derived || status != cluster.EdgeStatusConfirmed {
			continue
		}
		candidateRole, candidateDecidedBy, candidateConfidence := decisiveEntry(chain)
		candidateAutomatic := candidateRole == string(cluster.RoleSystem)
		if !ok || (!candidateAutomatic && automatic) {
			role, decidedBy, automatic, confidence, ok = candidateRole, candidateDecidedBy, candidateAutomatic, candidateConfidence, true
		}
	}
	return
}

// decisiveEntry picks the row that settled a CONFIRMED chain, same priority
// DeriveEdgeStatus uses: INCONSISTENCE is final if present, then a
// VERIFICATOR/REVIEWER agreement (the REVIEWER row, as the one that actually
// settled it), then SYSTEM alone. confidence always comes from the SYSTEM
// row when present, whether or not it was the decisive one -- a human
// confirming a system-proposed match doesn't erase the score that proposed
// it.
func decisiveEntry(rows []db.ListBiometricDecisionsForFeaturesRow) (role, decidedBy string, confidence *float64) {
	var sysRow, verRow, revRow, incRow *db.ListBiometricDecisionsForFeaturesRow
	for i := range rows {
		switch rows[i].Role {
		case string(cluster.RoleSystem):
			sysRow = &rows[i]
		case string(cluster.RoleVerificator):
			verRow = &rows[i]
		case string(cluster.RoleReviewer):
			revRow = &rows[i]
		case string(cluster.RoleInconsistence):
			incRow = &rows[i]
		}
	}

	pick := incRow
	if pick == nil {
		pick = revRow
	}
	if pick == nil {
		pick = verRow
	}
	if pick == nil {
		pick = sysRow
	}
	if pick == nil {
		return "", "", nil
	}

	decidedBy = pick.Username.String
	if pick.Role == string(cluster.RoleSystem) {
		decidedBy = pick.SystemSource.String
	}
	if sysRow != nil && sysRow.Confidence.Valid {
		v := sysRow.Confidence.Float64
		confidence = &v
	}
	return pick.Role, decidedBy, confidence
}
