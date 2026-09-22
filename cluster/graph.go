package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// Graph node kinds. A cluster node stands for the shared biometric feature
// that links an enrolled identity to the case traces it matched.
const (
	NodePerson  = "person"
	NodeCluster = "cluster"
	NodeTrace   = "trace"
)

// GraphNode is one vertex of the identity/cluster/trace graph.
type GraphNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	// Refs let the UI link a node back to its page.
	PersonID string `json:"personId,omitempty"`
	CaseID   string `json:"caseId,omitempty"`
	Modality string `json:"modality,omitempty"`
	// Size hints how many members a cluster node holds.
	Size int `json:"size,omitempty"`
}

// GraphEdge links two GraphNode ids.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// GraphResult is the node/edge set BuildGraph returns.
type GraphResult struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// BuildGraph renders the top `limit` clusters holding at least minMembers samples (by
// distinct cases touched, see ListOverview) as a graph: person -- cluster -- trace. Each cluster is one
// biometric feature group; KNOWN members attach it to the enrolled person,
// QUESTIONED members attach it to the case trace they came from.
func BuildGraph(ctx context.Context, sqlDB *sql.DB, limit, minMembers int) (GraphResult, error) {
	// ListOverview caps a page at 100, so gather larger limits page by page.
	const pageSize = 100
	var items []Summary
	for page := 1; len(items) < limit; page++ {
		overview, err := ListOverview(ctx, sqlDB, ListOverviewParams{Page: page, PageSize: pageSize, MinMembers: minMembers})
		if err != nil {
			return GraphResult{}, err
		}
		items = append(items, overview.Items...)
		if page >= overview.TotalPages {
			break
		}
	}
	if len(items) > limit {
		items = items[:limit]
	}
	out := GraphResult{Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	if len(items) == 0 {
		return out, nil
	}

	clusterIDs := make([]int64, 0, len(items))
	for _, s := range items {
		clusterIDs = append(clusterIDs, s.ClusterID)
	}
	q := db.New(sqlDB)
	members, err := q.ListClusterMembersByClusterIDs(ctx, clusterIDs)
	if err != nil {
		return GraphResult{}, fmt.Errorf("cluster: graph members: %w", err)
	}

	var traceIDs, fileIDs []int64
	traceCluster := map[int64][]int64{}
	fileCluster := map[int64][]int64{}
	for _, m := range members {
		if id, ok := graph.ParseQuestionedFeatureID(m.FeatureID); ok {
			traceIDs = append(traceIDs, id)
			traceCluster[id] = append(traceCluster[id], m.ClusterID)
		} else if id, err := strconv.ParseInt(m.FeatureID, 10, 64); err == nil {
			fileIDs = append(fileIDs, id)
			fileCluster[id] = append(fileCluster[id], m.ClusterID)
		}
	}

	clusterNode := func(id int64) string { return "cluster:" + strconv.FormatInt(id, 10) }
	for _, s := range items {
		out.Nodes = append(out.Nodes, GraphNode{
			ID:       clusterNode(s.ClusterID),
			Kind:     NodeCluster,
			Label:    fmt.Sprintf("Cluster #%d", s.ClusterID),
			Modality: s.Modality,
			Size:     s.MemberCount,
		})
	}

	seenNode := map[string]bool{}
	seenEdge := map[GraphEdge]bool{}
	link := func(source, target string) {
		e := GraphEdge{Source: source, Target: target}
		if !seenEdge[e] {
			seenEdge[e] = true
			out.Edges = append(out.Edges, e)
		}
	}

	if len(fileIDs) > 0 {
		rows, err := q.ListKnownClusterMembers(ctx, fileIDs)
		if err != nil {
			return GraphResult{}, fmt.Errorf("cluster: graph identities: %w", err)
		}
		for _, r := range rows {
			nodeID := "person:" + r.PersonID
			if !seenNode[nodeID] {
				seenNode[nodeID] = true
				out.Nodes = append(out.Nodes, GraphNode{ID: nodeID, Kind: NodePerson, Label: r.Name, PersonID: r.PersonID})
			}
			for _, cid := range fileCluster[r.IdentityFileID] {
				link(nodeID, clusterNode(cid))
			}
		}
	}

	if len(traceIDs) > 0 {
		rows, err := q.ListCasesByCaseTraceIDs(ctx, traceIDs)
		if err != nil {
			return GraphResult{}, fmt.Errorf("cluster: graph traces: %w", err)
		}
		for _, r := range rows {
			nodeID := "trace:" + strconv.FormatInt(r.CaseTraceID, 10)
			if !seenNode[nodeID] {
				seenNode[nodeID] = true
				out.Nodes = append(out.Nodes, GraphNode{
					ID:       nodeID,
					Kind:     NodeTrace,
					Label:    fmt.Sprintf("%s · trace %d", r.CaseID, r.CaseTraceID),
					CaseID:   r.CaseID,
					Modality: r.Modality,
				})
			}
			for _, cid := range traceCluster[r.CaseTraceID] {
				link(clusterNode(cid), nodeID)
			}
		}
	}
	return out, nil
}
