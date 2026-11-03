package articles

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/person"
)

// sharedCase is one case a person shares with an associate, and whether both links to it are
// reviewed.
type sharedCase struct {
	caseID    string
	confirmed bool
}

// personLinks is everything the records link to one person, at every level of certainty: the
// person's reviewed profile, each case with its certainty (caseLink.tier), the AUTOMATIC
// clusters holding the person and the other enrolments in them, and the persons linked to the
// same cases. The draft (PersonTemplate) and the counts (PersonLinkCounts) both read it.
type personLinks struct {
	profile        person.Profile
	cases          map[string]*caseLink
	caseOrder      []string
	byTier         map[string][]*caseLink
	autoClusters   []int64
	autoOthers     map[int64][]string // AUTOMATIC cluster -> other persons enrolled in it
	reviewedOthers map[int64][]string // REVIEWED cluster -> other persons enrolled in it
	associates     map[string][]sharedCase
	associateOrder []string
}

// gatherPersonLinks loads a person's links (personLinks). Returns ErrSubjectNotFound for an
// unknown person.
func gatherPersonLinks(ctx context.Context, sqlDB *sql.DB, personID string) (*personLinks, error) {
	profile, err := person.GetProfile(ctx, sqlDB, personID)
	if err != nil {
		if errors.Is(err, person.ErrNotFound) {
			return nil, ErrSubjectNotFound
		}
		return nil, err
	}
	q := db.New(sqlDB)
	pl := &personLinks{profile: profile, cases: map[string]*caseLink{}, byTier: map[string][]*caseLink{},
		autoOthers: map[int64][]string{}, reviewedOthers: map[int64][]string{}, associates: map[string][]sharedCase{}}

	// The person's enrolled features: where every link starts.
	var known []string
	own := map[string]bool{}
	for _, doc := range profile.Identity.Documents {
		for _, r := range doc.Registers {
			for _, f := range r.Files {
				if f.FeatureID != 0 {
					id := graph.KnownFeatureID(f.IdentityFileID)
					own[id] = true
					known = append(known, id)
				}
			}
		}
	}

	link := func(caseID string) *caseLink {
		if l, ok := pl.cases[caseID]; ok {
			return l
		}
		l := &caseLink{caseID: caseID, statuses: map[string]bool{}}
		pl.cases[caseID] = l
		pl.caseOrder = append(pl.caseOrder, caseID)
		return l
	}
	for _, c := range profile.Cases {
		link(c.CaseID).reviewed = true
	}
	decisions, err := cases.ListPersonTraceDecisions(ctx, sqlDB, known)
	if err != nil {
		return nil, err
	}
	for _, dec := range decisions {
		l := link(dec.CaseID)
		l.decisions = append(l.decisions, dec)
		l.statuses[dec.Status] = true
	}

	for _, c := range profile.Clusters {
		seen := map[string]bool{}
		for _, m := range c.Members {
			if m.Kind == "KNOWN" && m.PersonID != "" && m.PersonID != personID && !seen[m.PersonID] {
				seen[m.PersonID] = true
				pl.reviewedOthers[c.ClusterID] = append(pl.reviewedOthers[c.ClusterID], m.PersonID)
			}
		}
	}

	// The AUTOMATIC layer: every link the matcher made, reviewed or not, direct or through other
	// traces; and the other enrolments those clusters hold.
	if len(known) > 0 {
		pl.autoClusters, err = q.ListClusterIDsForFeatureIDs(ctx, db.ListClusterIDsForFeatureIDsParams{
			Basis: string(cluster.BasisAutomatic), FeatureIds: known})
		if err != nil {
			return nil, err
		}
	}
	if len(pl.autoClusters) > 0 {
		members, err := q.ListClusterMembersByClusterIDs(ctx, pl.autoClusters)
		if err != nil {
			return nil, err
		}
		var traceIDs []int64
		traceCluster := map[int64]int64{}
		var otherKnown []string
		otherCluster := map[string]int64{}
		for _, m := range members {
			if traceID, ok := graph.ParseQuestionedFeatureID(m.FeatureID); ok {
				traceIDs = append(traceIDs, traceID)
				traceCluster[traceID] = m.ClusterID
			} else if !own[m.FeatureID] {
				otherKnown = append(otherKnown, m.FeatureID)
				otherCluster[m.FeatureID] = m.ClusterID
			}
		}
		if len(traceIDs) > 0 {
			rows, err := q.ListCasesByCaseTraceIDs(ctx, traceIDs)
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				if l := link(r.CaseID); l.automatic == 0 {
					l.automatic = traceCluster[r.CaseTraceID]
				}
			}
		}
		if len(otherKnown) > 0 {
			described, err := cases.DescribeFeatures(ctx, q, otherKnown)
			if err != nil {
				return nil, err
			}
			seen := map[string]bool{}
			for _, f := range otherKnown {
				p, ok := described[f]
				if !ok || p.PersonID == "" || p.PersonID == personID || seen[p.PersonID] {
					continue
				}
				seen[p.PersonID] = true
				pl.autoOthers[otherCluster[f]] = append(pl.autoOthers[otherCluster[f]], p.PersonID)
			}
		}
	}

	// Associates: persons the AUTOMATIC layer links to the same cases (every case but the
	// excluded), each shared case confirmed only when both links are in the REVIEWED layer.
	var openCases, confirmedCases []string
	for _, id := range pl.caseOrder {
		l := pl.cases[id]
		t := l.tier()
		pl.byTier[t] = append(pl.byTier[t], l)
		if t != tierExcluded {
			openCases = append(openCases, id)
		}
		if t == tierConfirmed {
			confirmedCases = append(confirmedCases, id)
		}
	}
	if len(openCases) > 0 {
		confirmedPair := map[[2]string]bool{}
		if len(confirmedCases) > 0 {
			rows, err := q.ListPersonsLinkedToCases(ctx, db.ListPersonsLinkedToCasesParams{
				Basis: string(cluster.BasisReviewed), CaseIds: confirmedCases, PersonID: personID})
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				confirmedPair[[2]string{r.PersonID, r.CaseID}] = true
			}
		}
		rows, err := q.ListPersonsLinkedToCases(ctx, db.ListPersonsLinkedToCasesParams{
			Basis: string(cluster.BasisAutomatic), CaseIds: openCases, PersonID: personID})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if _, ok := pl.associates[r.PersonID]; !ok {
				pl.associateOrder = append(pl.associateOrder, r.PersonID)
			}
			pl.associates[r.PersonID] = append(pl.associates[r.PersonID],
				sharedCase{r.CaseID, confirmedPair[[2]string{r.PersonID, r.CaseID}]})
		}
	}
	return pl, nil
}

// LinkCounts is how much the records link to a person, by certainty: what the infobox shows
// beside the reviewed profile, so an analyst sees the matcher's unreviewed links as well.
type LinkCounts struct {
	// Cases by certainty: confirmed, unreviewed, uncertain, excluded (caseLink.tier).
	Cases map[string]int `json:"cases"`
	// Clusters holding the person in each layer.
	Clusters struct {
		Reviewed  int `json:"reviewed"`
		Automatic int `json:"automatic"`
	} `json:"clusters"`
	// OtherIdentities are other persons' enrolments grouped with this person's: confirmed when a
	// REVIEWED cluster holds them, unreviewed when only an AUTOMATIC one does.
	OtherIdentities struct {
		Confirmed  int `json:"confirmed"`
		Unreviewed int `json:"unreviewed"`
	} `json:"otherIdentities"`
	// Associates are persons linked to the same cases: confirmed when both links to at least
	// one shared case are reviewed.
	Associates struct {
		Confirmed  int `json:"confirmed"`
		Unreviewed int `json:"unreviewed"`
	} `json:"associates"`
}

// PersonLinkCounts counts a person's links by certainty (LinkCounts).
func PersonLinkCounts(ctx context.Context, sqlDB *sql.DB, personID string) (LinkCounts, error) {
	pl, err := gatherPersonLinks(ctx, sqlDB, personID)
	if err != nil {
		return LinkCounts{}, err
	}
	var out LinkCounts
	out.Cases = map[string]int{}
	for _, tier := range tierOrder {
		out.Cases[tier] = len(pl.byTier[tier])
	}
	out.Clusters.Reviewed = len(pl.profile.Clusters)
	out.Clusters.Automatic = len(pl.autoClusters)

	confirmedOther := map[string]bool{}
	for _, others := range pl.reviewedOthers {
		for _, p := range others {
			confirmedOther[p] = true
		}
	}
	unreviewedOther := map[string]bool{}
	for _, others := range pl.autoOthers {
		for _, p := range others {
			if !confirmedOther[p] {
				unreviewedOther[p] = true
			}
		}
	}
	out.OtherIdentities.Confirmed, out.OtherIdentities.Unreviewed = len(confirmedOther), len(unreviewedOther)

	for _, p := range pl.associateOrder {
		confirmed := false
		for _, s := range pl.associates[p] {
			confirmed = confirmed || s.confirmed
		}
		if confirmed {
			out.Associates.Confirmed++
		} else {
			out.Associates.Unreviewed++
		}
	}
	return out, nil
}
