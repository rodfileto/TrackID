package cluster

import "sort"

// edge is a confirmed comparison between two evidence items of one modality.
type edge struct {
	left     string
	right    string
	modality string
}

// clusterState is a persisted cluster's current shape.
type clusterState struct {
	modality string
	members  []string
}

// plan is the outcome of reconciling edges against existing clusters. It is a
// pure function of its inputs, so it is unit-testable without a database.
type plan struct {
	// kept maps each surviving cluster id to its final members.
	kept map[int64][]string
	// newClusters are clusters to create (groups of evidence with no surviving
	// existing cluster).
	newClusters []newCluster
	// merges records retired clusters: from was merged into to.
	merges []merge
}

type newCluster struct {
	modality string
	evidence []string
}

type merge struct {
	from int64
	to   int64
}

type unionFind struct {
	parent map[string]string
}

func newUnionFind() *unionFind {
	return &unionFind{parent: map[string]string{}}
}

func (u *unionFind) find(x string) string {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
	if u.parent[x] != x {
		u.parent[x] = u.find(u.parent[x])
	}
	return u.parent[x]
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

// reconcile computes the incremental clustering plan: how existing clusters
// extend, merge, split, and how new clusters form, given the confirmed edges.
//
// modalities maps evidence identifiers (biometric_cases.case_id) to their
// modality. edges are the confirmed comparisons. existing maps each persisted
// cluster id to its current case type and members.
//
// A cluster's identity follows its anchor — the smallest evidence id among its
// members — so when a rejected comparison splits a cluster, the component
// containing the anchor keeps the id and the rest form new clusters. When two
// clusters are joined by a new edge, the oldest id survives.
func reconcile(modalities map[string]string, edges []edge, existing map[int64]clusterState) plan {
	nodeModality := map[string]string{}
	for id, ct := range modalities {
		nodeModality[id] = ct
	}

	memberCluster := map[string]int64{}
	for id, cs := range existing {
		for _, m := range cs.members {
			memberCluster[m] = id
			if _, ok := nodeModality[m]; !ok {
				nodeModality[m] = cs.modality
			}
		}
	}
	for _, e := range edges {
		if _, ok := nodeModality[e.left]; !ok {
			nodeModality[e.left] = e.modality
		}
		if _, ok := nodeModality[e.right]; !ok {
			nodeModality[e.right] = e.modality
		}
	}

	uf := newUnionFind()
	for _, e := range edges {
		uf.union(e.left, e.right)
	}

	anchor := map[int64]string{}
	for id, cs := range existing {
		if len(cs.members) == 0 {
			continue
		}
		anchor[id] = cs.members[0]
		for _, m := range cs.members[1:] {
			if m < anchor[id] {
				anchor[id] = m
			}
		}
	}

	componentMembers := map[string][]string{}
	for evidence := range nodeModality {
		root := uf.find(evidence)
		componentMembers[root] = append(componentMembers[root], evidence)
	}

	present := map[string]map[int64]bool{}
	for evidence, cid := range memberCluster {
		root := uf.find(evidence)
		if present[root] == nil {
			present[root] = map[int64]bool{}
		}
		present[root][cid] = true
	}

	keptByComponent := map[string]int64{}
	for root, members := range componentMembers {
		memberSet := make(map[string]bool, len(members))
		for _, m := range members {
			memberSet[m] = true
		}
		var kept int64
		for cid := range present[root] {
			if a, ok := anchor[cid]; ok && memberSet[a] {
				if kept == 0 || cid < kept {
					kept = cid
				}
			}
		}
		keptByComponent[root] = kept
	}

	var p plan
	p.kept = map[int64][]string{}
	for root, members := range componentMembers {
		kept := keptByComponent[root]
		if kept != 0 {
			p.kept[kept] = append(p.kept[kept], members...)
			continue
		}
		sorted := append([]string(nil), members...)
		sort.Strings(sorted)
		p.newClusters = append(p.newClusters, newCluster{
			modality: nodeModality[sorted[0]],
			evidence: sorted,
		})
	}

	for root, members := range componentMembers {
		kept := keptByComponent[root]
		if kept == 0 {
			continue
		}
		memberSet := make(map[string]bool, len(members))
		for _, m := range members {
			memberSet[m] = true
		}
		for cid := range present[root] {
			if cid == kept {
				continue
			}
			if a, ok := anchor[cid]; ok && memberSet[a] {
				p.merges = append(p.merges, merge{from: cid, to: kept})
			}
		}
	}
	sort.Slice(p.merges, func(i, j int) bool { return p.merges[i].from < p.merges[j].from })
	for id := range p.kept {
		sort.Strings(p.kept[id])
	}
	return p
}
