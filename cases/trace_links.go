package cases

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"

	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// CaseCluster is one REVIEWED biometric cluster (see cluster.BasisReviewed and
// person.Cluster) that at least one of a case's codified traces resolved into:
// its members are linked only by decisions people confirmed, never by a SYSTEM
// match alone. Traces of the same case that share a cluster are merged into a
// single entry (LocalTraceIDs) rather than repeating the same cluster once per
// trace.
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
// case's trace, or an enrolled person's KNOWN feature.
//
// In ListCaseClusters every link is reviewed (cluster.Reviewed). Direct says
// how: true when a reviewed decision links the sample to one of the case's own
// traces (FromTraceID), described by DecisionRole/DecidedBy/Confidence; false
// when it is reached through other samples, every link of the way reviewed --
// Path names them, in order from FromTraceID, without the sample itself.
// FeatureID and FromFeatureID are the graph feature ids of the sample and of
// the case's trace its link starts from, so a caller can ask for a direct
// comparison of an indirect link (analyses.Open).
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
	Modality        string        `json:"modality,omitempty"`
	Description     string        `json:"description,omitempty"`
	TraceID         int64         `json:"traceId,omitempty"`
	ThumbnailFileID int64         `json:"thumbnailFileId,omitempty"`
	ThumbnailBox    *ThumbnailBox `json:"thumbnailBox,omitempty"`

	FeatureID     string     `json:"featureId,omitempty"`
	Direct        bool       `json:"direct"`
	FromTraceID   int64      `json:"fromTraceId,omitempty"`
	FromFeatureID string     `json:"fromFeatureId,omitempty"`
	Path          []PathStep `json:"path,omitempty"`

	DecisionRole string   `json:"decisionRole,omitempty"` // VERIFICATOR | REVIEWER | INCONSISTENCE
	DecidedBy    string   `json:"decidedBy,omitempty"`
	Confidence   *float64 `json:"confidence,omitempty"`
}

// PathStep is one sample an indirect TraceLink is reached through: another
// case's trace (CaseID, TraceID) or an enrolled person (PersonID, Name).
type PathStep struct {
	Kind     string `json:"kind"` // "KNOWN" | "QUESTIONED"
	CaseID   string `json:"caseId,omitempty"`
	TraceID  int64  `json:"traceId,omitempty"`
	PersonID string `json:"personId,omitempty"`
	Name     string `json:"name,omitempty"`
}

// ListCaseClusters resolves the distinct REVIEWED clusters a case's codified
// traces have resolved into, and the other cases/persons each one links to.
// The case page is where forensic analysis happens, so it reads only the
// REVIEWED layer: an automatic (SYSTEM-only) match never reaches it, not even
// as a count. Each link is direct (a reviewed decision with one of the case's
// traces) or indirect (a path of reviewed decisions), see TraceLink. Only
// clusters holding at least one reviewed pair are returned: a face alone in its
// reviewed cluster (an unknown subject no examiner has linked yet), or with no
// biometricfeature, contributes no entry. They come in the order their first
// trace appears in ListCaseCodifications, each with its direct links first.
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

	membershipRows, err := q.ListClusterMembershipsForFeatureIDs(ctx, db.ListClusterMembershipsForFeatureIDsParams{
		Basis: string(cluster.BasisReviewed), FeatureIds: featureIDs})
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
	clusterOfMember := map[string]int64{}
	var memberIDs []string
	memberRows, err := q.ListClusterMembersByClusterIDs(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}
	for _, m := range memberRows {
		membersByCluster[m.ClusterID] = append(membersByCluster[m.ClusterID], m.FeatureID)
		clusterOfMember[m.FeatureID] = m.ClusterID
		memberIDs = append(memberIDs, m.FeatureID)
	}
	described, err := DescribeFeatures(ctx, q, memberIDs)
	if err != nil {
		return nil, err
	}

	// Every decision touching a member, seen from the member's side:
	// chains[own][counterpart] is that pair's chain. A pair joins the reviewed
	// graph when people confirmed it and both sides share the cluster.
	decisionRows, err := q.ListBiometricDecisionsForFeatures(ctx, memberIDs)
	if err != nil {
		return nil, err
	}
	chains := map[string]map[string][]db.ListBiometricDecisionsForFeaturesRow{}
	for _, r := range decisionRows {
		if chains[r.FeatureID] == nil {
			chains[r.FeatureID] = map[string][]db.ListBiometricDecisionsForFeaturesRow{}
		}
		chains[r.FeatureID][r.CounterpartID] = append(chains[r.FeatureID][r.CounterpartID], r)
	}
	reviewedEdges := map[string][]string{}
	for own, byCounterpart := range chains {
		for counterpart, chain := range byCounterpart {
			if clusterOfMember[counterpart] != clusterOfMember[own] || !cluster.Reviewed(chainEntries(chain)) {
				continue
			}
			reviewedEdges[own] = append(reviewedEdges[own], counterpart)
		}
	}
	for own := range reviewedEdges {
		slices.Sort(reviewedEdges[own]) // a stable path when several are equally short
	}

	out := make([]CaseCluster, 0, len(clusterOrder))
	for _, clusterID := range clusterOrder {
		localTraceIDs := localTraces[clusterID]
		localFeatures := make([]string, len(localTraceIDs))
		isLocal := make(map[string]bool, len(localTraceIDs))
		for i, tid := range localTraceIDs {
			localFeatures[i] = graph.QuestionedFeatureID(tid)
			isLocal[localFeatures[i]] = true
		}
		members := membersByCluster[clusterID]
		paths := shortestPaths(localFeatures, reviewedEdges)

		var links []TraceLink
		for _, m := range members {
			if isLocal[m] {
				continue // one of this case's own faces, listed via LocalTraceIDs instead
			}
			link, ok := described[m]
			if !ok {
				continue // e.g. the trace's evidence was excluded/deleted since clustering
			}
			path, reached := paths[m]
			if !reached {
				// Clusters lag behind decisions until the next cluster.Run: a member no
				// reviewed path reaches any more is left out rather than shown unbacked.
				continue
			}
			link.FeatureID = m
			link.FromTraceID = traceOfFeature[path[0]]
			link.FromFeatureID = path[0]
			if len(path) == 2 {
				link.Direct = true
				link.DecisionRole, link.DecidedBy, link.Confidence = reviewedDecision(chains[path[0]][m])
			} else {
				for _, step := range path[1 : len(path)-1] {
					link.Path = append(link.Path, pathStep(described[step]))
				}
			}
			links = append(links, link)
		}
		if len(links) == 0 && len(localTraceIDs) < 2 {
			continue // a face no reviewed decision links to anything: nothing to show
		}
		// Direct links first, then indirect ones from the shortest path up.
		slices.SortStableFunc(links, func(a, b TraceLink) int { return len(a.Path) - len(b.Path) })

		out = append(out, CaseCluster{
			ClusterID:     clusterID,
			MemberCount:   len(members),
			LocalTraceIDs: localTraceIDs,
			Links:         links,
		})
	}
	return out, nil
}

// shortestPaths runs a breadth-first search over edges from every one of sources at once and
// returns, for each feature reached, a shortest path to it: its first element is the source it
// starts from, its last the feature itself. Sources are tried in order and each node's neighbours
// in edges order, so equally short paths resolve the same way every time. Sources map to
// themselves (a path of one).
func shortestPaths(sources []string, edges map[string][]string) map[string][]string {
	paths := make(map[string][]string, len(sources))
	queue := make([]string, 0, len(sources))
	for _, s := range sources {
		if _, seen := paths[s]; seen {
			continue
		}
		paths[s] = []string{s}
		queue = append(queue, s)
	}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, next := range edges[node] {
			if _, seen := paths[next]; seen {
				continue
			}
			paths[next] = append(append(make([]string, 0, len(paths[node])+1), paths[node]...), next)
			queue = append(queue, next)
		}
	}
	return paths
}

// chainEntries converts one pair's rows into the chain cluster.Reviewed and
// cluster.DecidingRound read.
func chainEntries(chain []db.ListBiometricDecisionsForFeaturesRow) []cluster.ChainEntry {
	entries := make([]cluster.ChainEntry, len(chain))
	for i, r := range chain {
		entries[i] = cluster.ChainEntry{Role: cluster.Role(r.Role), Decision: cluster.Decision(r.Decision), Analysis: r.CaseAnalysisID.Int64}
	}
	return entries
}

// reviewedDecision describes the decision that settled a reviewed pair: the decisive row of its
// deciding round (decisiveEntry).
func reviewedDecision(chain []db.ListBiometricDecisionsForFeaturesRow) (role, decidedBy string, confidence *float64) {
	entries := chainEntries(chain)
	round, _, ok := cluster.DecidingRound(entries)
	if !ok {
		return "", "", nil
	}
	var current []db.ListBiometricDecisionsForFeaturesRow
	for i, r := range chain {
		if entries[i].Counts(round) {
			current = append(current, r)
		}
	}
	return decisiveEntry(current)
}

// pathStep names a described sample as a step of an indirect link's path.
func pathStep(l TraceLink) PathStep {
	return PathStep{Kind: l.Kind, CaseID: l.CaseID, TraceID: l.TraceID, PersonID: l.PersonID, Name: l.Name}
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

// DescribeFeatures resolves graph feature ids into what a caller needs to show them: for a KNOWN
// feature, its enrolled person and identity file; for a QUESTIONED one, its case and how to render
// its thumbnail (see TraceLink). Only the descriptive fields are set, none of the decision ones. An
// id that resolves to nothing (malformed, or its evidence since excluded) is left out.
func DescribeFeatures(ctx context.Context, q *db.Queries, featureIDs []string) (map[string]TraceLink, error) {
	var identityFileIDs, traceIDs []int64
	seen := map[string]bool{}
	for _, id := range featureIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if tid, ok := graph.ParseQuestionedFeatureID(id); ok {
			traceIDs = append(traceIDs, tid)
		} else if fid, err := strconv.ParseInt(id, 10, 64); err == nil {
			identityFileIDs = append(identityFileIDs, fid)
		}
	}

	out := map[string]TraceLink{}
	if len(identityFileIDs) > 0 {
		rows, err := q.ListKnownClusterMembers(ctx, identityFileIDs)
		if err != nil {
			return nil, err
		}
		for _, d := range rows {
			out[graph.KnownFeatureID(d.IdentityFileID)] = TraceLink{Kind: "KNOWN", PersonID: d.PersonID, Name: d.Name,
				IdentityFileID: d.IdentityFileID, ContentType: d.ContentType.String}
		}
	}
	if len(traceIDs) > 0 {
		rows, err := q.ListQuestionedClusterMembers(ctx, traceIDs)
		if err != nil {
			return nil, err
		}
		for _, d := range rows {
			link := TraceLink{Kind: "QUESTIONED", CaseID: d.CaseID, Modality: d.Modality, Description: d.Description, TraceID: d.CaseTraceID}
			switch {
			case d.TraceCropFileID.Valid:
				link.ThumbnailFileID = d.TraceCropFileID.Int64
			case d.EvidenceFileID.Valid && d.BoxX1.Valid:
				link.ThumbnailFileID = d.EvidenceFileID.Int64
				link.ThumbnailBox = &ThumbnailBox{X1: d.BoxX1.Float64, Y1: d.BoxY1.Float64, X2: d.BoxX2.Float64, Y2: d.BoxY2.Float64}
			}
			out[graph.QuestionedFeatureID(d.CaseTraceID)] = link
		}
	}
	return out, nil
}
