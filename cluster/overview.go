package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// Summary is one persisted cluster's row in a general clusters listing: how
// many biometric samples it holds, how many distinct criminal cases its
// QUESTIONED members touch, and whether it has resolved to an enrolled
// identity (see Identify).
type Summary struct {
	ClusterID   int64           `json:"clusterId"`
	CaseType    string          `json:"caseType"`
	CreatedAt   time.Time       `json:"createdAt"`
	MemberCount int             `json:"memberCount"`
	CaseCount   int             `json:"caseCount"`
	Identified  bool            `json:"identified"`
	Persons     []SummaryPerson `json:"persons,omitempty"`
}

// SummaryPerson is one enrolled person a Summary's cluster has resolved to.
type SummaryPerson struct {
	PersonID string `json:"personId"`
	Name     string `json:"name"`
}

// ListOverviewParams filters/paginates ListOverview.
type ListOverviewParams struct {
	Page     int
	PageSize int
	// IdentifiedOnly keeps only clusters with at least one KNOWN member --
	// resolved to an enrolled identity, not just to other case evidence.
	IdentifiedOnly bool
	// MinMembers keeps only clusters holding at least this many samples.
	MinMembers int
}

// ListOverviewResult is ListOverview's paginated result.
type ListOverviewResult struct {
	Items      []Summary
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

// ListOverview lists every persisted cluster (cluster.Run's output), ordered
// by how many distinct criminal cases it touches (descending) -- the
// clusters most relevant to active investigations surface first. Filtered
// and paginated at the database level; safe to call on every request even
// against a large cluster table.
func ListOverview(ctx context.Context, sqlDB *sql.DB, params ListOverviewParams) (ListOverviewResult, error) {
	if sqlDB == nil {
		return ListOverviewResult{}, fmt.Errorf("database is not configured")
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var conds []string
	if params.IdentifiedOnly {
		conds = append(conds, "known_count > 0")
	}
	if params.MinMembers > 1 {
		conds = append(conds, fmt.Sprintf("member_count >= %d", params.MinMembers))
	}
	having := ""
	if len(conds) > 0 {
		having = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	countQuery := `
		WITH agg AS (
			SELECT c.id,
				COUNT(cm.feature_id) AS member_count,
				COUNT(*) FILTER (WHERE cm.feature_id NOT LIKE 'TRACE:%') AS known_count
			FROM clusters c
			JOIN cluster_members cm ON cm.cluster_id = c.id
			GROUP BY c.id
		)
		SELECT COUNT(*) FROM agg ` + having
	if err := sqlDB.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return ListOverviewResult{}, fmt.Errorf("cluster: count overview: %w", err)
	}

	listQuery := `
		WITH agg AS (
			SELECT
				c.id AS cluster_id,
				c.case_type,
				c.created_at,
				COUNT(cm.feature_id) AS member_count,
				COUNT(*) FILTER (WHERE cm.feature_id NOT LIKE 'TRACE:%') AS known_count,
				COUNT(DISTINCT cc.id) AS case_count
			FROM clusters c
			JOIN cluster_members cm ON cm.cluster_id = c.id
			LEFT JOIN case_traces ct ON cm.feature_id = 'TRACE:' || ct.id || '#feature'
			LEFT JOIN case_evidences ce ON ce.id = ct.evidence_id
			LEFT JOIN criminal_cases cc ON cc.id = ce.criminal_case_id
			GROUP BY c.id
		)
		SELECT cluster_id, case_type, created_at, member_count, case_count, known_count
		FROM agg ` + having + `
		ORDER BY case_count DESC, cluster_id DESC
		LIMIT $1 OFFSET $2`

	rows, err := sqlDB.QueryContext(ctx, listQuery, pageSize, (page-1)*pageSize)
	if err != nil {
		return ListOverviewResult{}, fmt.Errorf("cluster: list overview: %w", err)
	}
	defer rows.Close()

	items := []Summary{}
	var identifiedIDs []int64
	for rows.Next() {
		var s Summary
		var knownCount int
		if err := rows.Scan(&s.ClusterID, &s.CaseType, &s.CreatedAt, &s.MemberCount, &s.CaseCount, &knownCount); err != nil {
			return ListOverviewResult{}, fmt.Errorf("cluster: scan overview row: %w", err)
		}
		s.Identified = knownCount > 0
		if s.Identified {
			identifiedIDs = append(identifiedIDs, s.ClusterID)
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return ListOverviewResult{}, err
	}

	if len(identifiedIDs) > 0 {
		persons, err := loadIdentifiedPersons(ctx, sqlDB, identifiedIDs)
		if err != nil {
			return ListOverviewResult{}, err
		}
		for i := range items {
			items[i].Persons = persons[items[i].ClusterID]
		}
	}

	totalPages := (total + pageSize - 1) / pageSize
	return ListOverviewResult{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

// loadIdentifiedPersons resolves the enrolled persons behind each of the
// given clusters' KNOWN members, for ListOverview's Persons field. A cluster
// can resolve to more than one person only when two enrollments' features
// were both confirmed into it (e.g. a duplicate enrollment).
func loadIdentifiedPersons(ctx context.Context, sqlDB *sql.DB, clusterIDs []int64) (map[int64][]SummaryPerson, error) {
	q := db.New(sqlDB)
	memberRows, err := q.ListClusterMembersByClusterIDs(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}

	var identityFileIDs []int64
	fileCluster := map[int64]int64{}
	for _, m := range memberRows {
		if _, questioned := graph.ParseQuestionedFeatureID(m.FeatureID); questioned {
			continue
		}
		id, err := strconv.ParseInt(m.FeatureID, 10, 64)
		if err != nil {
			continue
		}
		identityFileIDs = append(identityFileIDs, id)
		fileCluster[id] = m.ClusterID
	}
	if len(identityFileIDs) == 0 {
		return nil, nil
	}

	knownRows, err := q.ListKnownClusterMembers(ctx, identityFileIDs)
	if err != nil {
		return nil, err
	}

	out := map[int64][]SummaryPerson{}
	seen := map[int64]map[string]bool{}
	for _, r := range knownRows {
		clusterID := fileCluster[r.IdentityFileID]
		if seen[clusterID] == nil {
			seen[clusterID] = map[string]bool{}
		}
		if seen[clusterID][r.PersonID] {
			continue
		}
		seen[clusterID][r.PersonID] = true
		out[clusterID] = append(out[clusterID], SummaryPerson{PersonID: r.PersonID, Name: r.Name})
	}
	return out, nil
}
