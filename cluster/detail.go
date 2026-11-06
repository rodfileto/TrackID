package cluster

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/rodfileto/trackid/graph"
)

// ErrClusterNotFound is returned by GetDetail when no cluster has the id.
var ErrClusterNotFound = errors.New("cluster: cluster not found")

// Detail is one cluster in depth, for the cluster page: what holds it together (the pairs that
// are its edges in its layer, which of them are bridges, each member's degree), every decision
// touching it, the merges into and out of it, where its members sit in the other layer, and
// where the enrolled persons it resolves to keep their other samples.
type Detail struct {
	ClusterID int64     `json:"clusterId"`
	Modality  string    `json:"modality"`
	Basis     Basis     `json:"basis"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	Members []DetailMember `json:"members"`
	// Pairs are every decided pair with at least one member in the cluster: Inside pairs (both
	// members) first, then boundary pairs (one member, one outside).
	Pairs []DetailPair `json:"pairs"`
	// Components is how many groups the cluster's own edges split its members into. A cluster
	// Run has just written is 1 (a singleton included); more means decisions changed since.
	Components int           `json:"components"`
	Merges     []DetailMerge `json:"merges"`
	// PersonSamples are every enrollment sample, of any modality, of every person a KNOWN member
	// resolves to, with the cluster of this layer each one is in: a person whose samples of the
	// cluster's modality sit in different clusters holds biometrics that did not match each other.
	PersonSamples []PersonSample `json:"personSamples"`
	// LinkedClusters are the clusters of this layer, of another modality, holding those persons'
	// samples. They are linked by enrollment (one register holds both samples), not by a
	// comparison, so nothing here joins them: each modality's cluster stands on its own decisions.
	LinkedClusters []LinkedCluster `json:"linkedClusters"`
}

// LinkedCluster is a cluster of another modality that holds an enrollment sample of a person the
// cluster resolves to.
type LinkedCluster struct {
	ClusterID int64  `json:"clusterId"`
	Modality  string `json:"modality"`
	// MemberCount and QuestionedCount are its members, and those of them from case evidence.
	MemberCount     int `json:"memberCount"`
	QuestionedCount int `json:"questionedCount"`
	// PersonIDs are the persons whose samples link it.
	PersonIDs []string `json:"personIds"`
}

// DetailMember is one member's place in its cluster's structure.
type DetailMember struct {
	FeatureID string `json:"featureId"`
	// Degree is how many of the cluster's edges touch the member.
	Degree int `json:"degree"`
	// CutVertex is true when removing the member would split the cluster.
	CutVertex bool `json:"cutVertex"`
	// OtherClusterID is the member's cluster in the other layer (0 when it has none).
	OtherClusterID int64 `json:"otherClusterId,omitempty"`
}

// DetailPair is one decided pair touching the cluster, with its full decision chain.
type DetailPair struct {
	FeatureA string `json:"featureA"`
	FeatureB string `json:"featureB"`
	// Inside is true when both features are members; otherwise one is outside the cluster, and
	// OutsideClusterID is that feature's cluster in the same layer (0 when it has none).
	Inside           bool  `json:"inside"`
	OutsideClusterID int64 `json:"outsideClusterId,omitempty"`
	// Status and SettledBy are the pair's deciding round (DecidingRoundBy); Reviewed is whether
	// people confirmed it (Reviewed).
	Status    EdgeStatus `json:"status"`
	SettledBy Role       `json:"settledBy,omitempty"`
	Reviewed  bool       `json:"reviewed"`
	// Edge is true when the pair links its features in the cluster's layer: CONFIRMED for
	// AUTOMATIC, Reviewed for REVIEWED (edgesFor).
	Edge bool `json:"edge"`
	// Bridge is true for an edge whose removal would split the cluster: the only link between
	// two groups of members.
	Bridge bool `json:"bridge"`
	// Score and Threshold are the SYSTEM decision's, when the pair has one.
	Score     *float64         `json:"score,omitempty"`
	Threshold *float64         `json:"threshold,omitempty"`
	Chain     []DetailDecision `json:"chain"`
}

// DetailDecision is one biometric_decisions row of a pair's chain.
type DetailDecision struct {
	Role         Role      `json:"role"`
	Decision     Decision  `json:"decision"`
	DecidedBy    string    `json:"decidedBy"` // username, or system_source for SYSTEM
	Confidence   *float64  `json:"confidence,omitempty"`
	Threshold    *float64  `json:"threshold,omitempty"`
	AnalysisID   int64     `json:"analysisId,omitempty"`
	AnalysisType string    `json:"analysisType,omitempty"`
	Conclusion   string    `json:"conclusion,omitempty"`
	DecidedAt    time.Time `json:"decidedAt"`
}

// DetailMerge is one cluster_merges row naming the cluster, either side.
type DetailMerge struct {
	FromClusterID int64     `json:"fromClusterId"`
	ToClusterID   int64     `json:"toClusterId"`
	MergedAt      time.Time `json:"mergedAt"`
	Reason        string    `json:"reason,omitempty"`
}

// PersonSample is one enrollment sample of a person the cluster resolves to.
type PersonSample struct {
	PersonID       string `json:"personId"`
	PersonNumber   int64  `json:"personNumber"`
	Name           string `json:"name"`
	RegisterNumber string `json:"registerNumber"`
	// BirthDate and the parents' names are the sample's register's, for comparing the persons on the page.
	BirthDate      string `json:"birthDate,omitempty"`
	Parent1Name    string `json:"parent1Name"`
	Parent2Name    string `json:"parent2Name"`
	IdentityFileID int64  `json:"identityFileId"`
	FileType       string `json:"fileType"`
	Modality       string `json:"modality"`
	ContentType    string `json:"contentType,omitempty"`
	FeatureID      string `json:"featureId"`
	// ClusterID is the sample's cluster in the same layer (0 when it has none).
	ClusterID int64 `json:"clusterId,omitempty"`
}

// GetDetail returns one cluster, of either layer, in depth (Detail). Returns ErrClusterNotFound
// when no cluster has the id.
func GetDetail(ctx context.Context, sqlDB *sql.DB, clusterID int64) (Detail, error) {
	if sqlDB == nil {
		return Detail{}, fmt.Errorf("database is not configured")
	}
	var d Detail
	var basis string
	err := sqlDB.QueryRowContext(ctx,
		`SELECT id, modality, basis, created_at, updated_at FROM clusters WHERE id = $1`, clusterID).
		Scan(&d.ClusterID, &d.Modality, &basis, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Detail{}, ErrClusterNotFound
	}
	if err != nil {
		return Detail{}, fmt.Errorf("cluster: load cluster: %w", err)
	}
	d.Basis = Basis(basis)

	members, err := loadMemberIDs(ctx, sqlDB, clusterID)
	if err != nil {
		return Detail{}, err
	}
	memberSet := make(map[string]bool, len(members))
	for _, m := range members {
		memberSet[m] = true
	}

	if d.Pairs, err = loadDetailPairs(ctx, sqlDB, members, memberSet, d.Basis); err != nil {
		return Detail{}, err
	}
	other, err := layerClusters(ctx, sqlDB, members, otherBasis(d.Basis))
	if err != nil {
		return Detail{}, err
	}
	var outside []string
	for _, p := range d.Pairs {
		if !p.Inside {
			outside = append(outside, outsideOf(p, memberSet))
		}
	}
	outsideCluster, err := layerClusters(ctx, sqlDB, outside, d.Basis)
	if err != nil {
		return Detail{}, err
	}
	for i := range d.Pairs {
		if !d.Pairs[i].Inside {
			d.Pairs[i].OutsideClusterID = outsideCluster[outsideOf(d.Pairs[i], memberSet)]
		}
	}
	d.Members, d.Components = structure(members, d.Pairs)
	for i := range d.Members {
		d.Members[i].OtherClusterID = other[d.Members[i].FeatureID]
	}
	if d.Merges, err = loadMerges(ctx, sqlDB, clusterID, d.Basis); err != nil {
		return Detail{}, err
	}
	if d.PersonSamples, err = loadPersonSamples(ctx, sqlDB, clusterID, d.Basis); err != nil {
		return Detail{}, err
	}
	if d.LinkedClusters, err = loadLinkedClusters(ctx, sqlDB, d.Modality, d.PersonSamples); err != nil {
		return Detail{}, err
	}
	return d, nil
}

func loadMemberIDs(ctx context.Context, sqlDB *sql.DB, clusterID int64) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT feature_id FROM cluster_members WHERE cluster_id = $1 ORDER BY feature_id`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("cluster: load members: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// loadDetailPairs reads every decision touching a member, groups them by pair and derives each
// pair's status as Run does.
func loadDetailPairs(ctx context.Context, sqlDB *sql.DB, members []string, memberSet map[string]bool, basis Basis) ([]DetailPair, error) {
	if len(members) == 0 {
		return []DetailPair{}, nil
	}
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT d.feature_a_id, d.feature_b_id, d.role, d.decision,
		       COALESCE(d.username, d.system_source, ''), d.confidence, d.threshold,
		       COALESCE(d.case_analysis_id, 0), COALESCE(ca.analysis_type, ''), COALESCE(d.conclusion, ''),
		       d.decided_at
		FROM biometric_decisions d
		LEFT JOIN case_analyses ca ON ca.id = d.case_analysis_id
		WHERE d.feature_a_id = ANY($1::text[]) OR d.feature_b_id = ANY($1::text[])
		ORDER BY d.feature_a_id, d.feature_b_id, d.decided_at, d.id`, pq.Array(members))
	if err != nil {
		return nil, fmt.Errorf("cluster: load decisions: %w", err)
	}
	defer rows.Close()

	type key struct{ a, b string }
	chains := map[key][]DetailDecision{}
	var order []key
	for rows.Next() {
		var k key
		var dd DetailDecision
		var role, decision string
		var conf, thr sql.NullFloat64
		if err := rows.Scan(&k.a, &k.b, &role, &decision, &dd.DecidedBy, &conf, &thr,
			&dd.AnalysisID, &dd.AnalysisType, &dd.Conclusion, &dd.DecidedAt); err != nil {
			return nil, err
		}
		dd.Role, dd.Decision = Role(role), Decision(decision)
		if conf.Valid {
			dd.Confidence = &conf.Float64
		}
		if thr.Valid {
			dd.Threshold = &thr.Float64
		}
		if _, ok := chains[k]; !ok {
			order = append(order, k)
		}
		chains[k] = append(chains[k], dd)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pairs := make([]DetailPair, 0, len(order))
	for _, k := range order {
		ch := chains[k]
		entries := make([]ChainEntry, len(ch))
		p := DetailPair{FeatureA: k.a, FeatureB: k.b, Inside: memberSet[k.a] && memberSet[k.b], Chain: ch}
		for i, dd := range ch {
			entries[i] = ChainEntry{Role: dd.Role, Decision: dd.Decision, Analysis: dd.AnalysisID,
				AnalysisType: dd.AnalysisType, Conclusion: Conclusion(dd.Conclusion)}
			if dd.Role == RoleSystem {
				p.Score, p.Threshold = dd.Confidence, dd.Threshold
			}
		}
		_, p.Status, p.SettledBy, _ = DecidingRoundBy(entries)
		p.Reviewed = Reviewed(entries)
		if basis == BasisReviewed {
			p.Edge = p.Inside && p.Reviewed
		} else {
			p.Edge = p.Inside && p.Status == EdgeStatusConfirmed
		}
		pairs = append(pairs, p)
	}
	// Inside pairs first, weakest score first: the links most worth a second look lead.
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].Inside != pairs[j].Inside {
			return pairs[i].Inside
		}
		return scoreOf(pairs[i]) < scoreOf(pairs[j])
	})
	return pairs, nil
}

func scoreOf(p DetailPair) float64 {
	if p.Score == nil {
		return 2 // no SYSTEM score: after every scored pair
	}
	return *p.Score
}

// structure computes each member's degree and whether it is a cut vertex, marks the bridges
// among pairs' edges, and counts the components the edges split the members into.
func structure(members []string, pairs []DetailPair) ([]DetailMember, int) {
	index := make(map[string]int, len(members))
	for i, m := range members {
		index[m] = i
	}
	type adj struct{ to, pair int }
	g := make([][]adj, len(members))
	for pi, p := range pairs {
		if !p.Edge {
			continue
		}
		a, b := index[p.FeatureA], index[p.FeatureB]
		g[a] = append(g[a], adj{b, pi})
		g[b] = append(g[b], adj{a, pi})
	}

	// Tarjan's bridges and articulation points (recursive: a cluster is small).
	disc := make([]int, len(members))
	low := make([]int, len(members))
	cut := make([]bool, len(members))
	timer := 0
	var dfs func(u, parentPair int)
	dfs = func(u, parentPair int) {
		timer++
		disc[u], low[u] = timer, timer
		children := 0
		for _, e := range g[u] {
			if e.pair == parentPair {
				continue
			}
			if disc[e.to] != 0 {
				low[u] = min(low[u], disc[e.to])
				continue
			}
			children++
			dfs(e.to, e.pair)
			low[u] = min(low[u], low[e.to])
			if low[e.to] > disc[u] {
				pairs[e.pair].Bridge = true
			}
			if parentPair >= 0 && low[e.to] >= disc[u] {
				cut[u] = true
			}
		}
		if parentPair < 0 && children > 1 {
			cut[u] = true
		}
	}
	components := 0
	for i := range members {
		if disc[i] == 0 {
			components++
			dfs(i, -1)
		}
	}

	out := make([]DetailMember, len(members))
	for i, m := range members {
		out[i] = DetailMember{FeatureID: m, Degree: len(g[i]), CutVertex: cut[i]}
	}
	return out, components
}

func otherBasis(basis Basis) Basis {
	if basis == BasisReviewed {
		return BasisAutomatic
	}
	return BasisReviewed
}

// outsideOf is a boundary pair's feature that is not a member.
func outsideOf(p DetailPair, memberSet map[string]bool) string {
	if memberSet[p.FeatureA] {
		return p.FeatureB
	}
	return p.FeatureA
}

// layerClusters maps each of the features to its cluster in the layer.
func layerClusters(ctx context.Context, sqlDB *sql.DB, features []string, basis Basis) (map[string]int64, error) {
	out := map[string]int64{}
	if len(features) == 0 {
		return out, nil
	}
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT feature_id, cluster_id FROM cluster_members WHERE basis = $1 AND feature_id = ANY($2::text[])`,
		string(basis), pq.Array(features))
	if err != nil {
		return nil, fmt.Errorf("cluster: load other layer: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		var id int64
		if err := rows.Scan(&f, &id); err != nil {
			return nil, err
		}
		out[f] = id
	}
	return out, rows.Err()
}

func loadMerges(ctx context.Context, sqlDB *sql.DB, clusterID int64, basis Basis) ([]DetailMerge, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT from_cluster_id, to_cluster_id, merged_at, COALESCE(reason, '')
		FROM cluster_merges
		WHERE basis = $2 AND (from_cluster_id = $1 OR to_cluster_id = $1)
		ORDER BY merged_at, id`, clusterID, string(basis))
	if err != nil {
		return nil, fmt.Errorf("cluster: load merges: %w", err)
	}
	defer rows.Close()
	out := []DetailMerge{}
	for rows.Next() {
		var m DetailMerge
		if err := rows.Scan(&m.FromClusterID, &m.ToClusterID, &m.MergedAt, &m.Reason); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// loadPersonSamples lists every enrollment sample, of any modality, of the persons its KNOWN
// members resolve to, with each sample's cluster in the same layer.
func loadPersonSamples(ctx context.Context, sqlDB *sql.DB, clusterID int64, basis Basis) ([]PersonSample, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		WITH persons_in AS (
			SELECT DISTINCT d.person_id
			FROM cluster_members cm
			JOIN identity_files f ON cm.feature_id = f.id::text
			JOIN identity_registers r ON r.id = f.register_id
			JOIN identity_documents d ON d.id = r.document_id
			WHERE cm.cluster_id = $1 AND cm.feature_id NOT LIKE 'TRACE:%'
		)
		SELECT p.person_id, p.id, r.name, r.register_number, COALESCE(r.birth_date, ''), r.parent_1_name, r.parent_2_name,
		       f.id, f.file_type, COALESCE(f.content_type, ''),
		       bf.feature_type, COALESCE(cm.cluster_id, 0)
		FROM persons_in pi
		JOIN persons p ON p.id = pi.person_id
		JOIN identity_documents d ON d.person_id = p.id
		JOIN identity_registers r ON r.document_id = d.id
		JOIN identity_files f ON f.register_id = r.id
		JOIN biometric_features bf ON bf.identity_file_id = f.id
		LEFT JOIN cluster_members cm ON cm.basis = $2 AND cm.feature_id = f.id::text
		ORDER BY p.person_id, r.register_number, f.id`, clusterID, string(basis))
	if err != nil {
		return nil, fmt.Errorf("cluster: load person samples: %w", err)
	}
	defer rows.Close()
	out := []PersonSample{}
	for rows.Next() {
		var s PersonSample
		var featureType string
		if err := rows.Scan(&s.PersonID, &s.PersonNumber, &s.Name, &s.RegisterNumber, &s.BirthDate, &s.Parent1Name, &s.Parent2Name, &s.IdentityFileID, &s.FileType,
			&s.ContentType, &featureType, &s.ClusterID); err != nil {
			return nil, err
		}
		m, ok := graph.ModalityForFeatureType(featureType)
		if !ok {
			continue
		}
		s.Modality = m
		s.FeatureID = graph.KnownFeatureID(s.IdentityFileID)
		out = append(out, s)
	}
	return out, rows.Err()
}

// loadLinkedClusters gathers, from the person samples, the clusters of a modality other than the
// cluster's (LinkedCluster), with their member counts.
func loadLinkedClusters(ctx context.Context, sqlDB *sql.DB, modality string, samples []PersonSample) ([]LinkedCluster, error) {
	out := []LinkedCluster{}
	index := map[int64]int{}
	var ids []int64
	for _, s := range samples {
		if s.ClusterID == 0 || strings.EqualFold(s.Modality, modality) {
			continue
		}
		i, ok := index[s.ClusterID]
		if !ok {
			i = len(out)
			index[s.ClusterID] = i
			ids = append(ids, s.ClusterID)
			out = append(out, LinkedCluster{ClusterID: s.ClusterID, Modality: s.Modality})
		}
		if !slices.Contains(out[i].PersonIDs, s.PersonID) {
			out[i].PersonIDs = append(out[i].PersonIDs, s.PersonID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT cluster_id, COUNT(*), COUNT(*) FILTER (WHERE feature_id LIKE 'TRACE:%')
		FROM cluster_members WHERE cluster_id = ANY($1::bigint[]) GROUP BY cluster_id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("cluster: load linked clusters: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var members, questioned int
		if err := rows.Scan(&id, &members, &questioned); err != nil {
			return nil, err
		}
		out[index[id]].MemberCount, out[index[id]].QuestionedCount = members, questioned
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ClusterID < out[j].ClusterID })
	return out, nil
}
