package follows

import (
	"context"
	"database/sql"
	"log"
	"slices"
	"strconv"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/notifications"
	"github.com/rodfileto/trackid/realtime"
)

const (
	// pollInterval is how often the fan-out looks for changes when no signal woke it, in case one was missed.
	pollInterval = 30 * time.Second
	// pruneInterval and retention bound how long handled changes are kept.
	pruneInterval = 24 * time.Hour
	retention     = 90 * 24 * time.Hour
)

// Run fans out changes until ctx ends: on each signal on Channel, and every pollInterval. It belongs in one process,
// the API's; a second would only wait on the lock.
func Run(ctx context.Context, sqlDB *sql.DB, databaseURL string) {
	wake := make(chan struct{}, 1)
	poke := func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	go realtime.Listen(ctx, databaseURL, Channel, func(context.Context, string) { poke() }, poke)
	poll := time.NewTicker(pollInterval)
	defer poll.Stop()
	prune := time.NewTicker(pruneInterval)
	defer prune.Stop()
	poke()
	for {
		select {
		case <-ctx.Done():
			return
		case <-prune.C:
			if _, err := Prune(ctx, sqlDB, time.Now().Add(-retention)); err != nil && ctx.Err() == nil {
				log.Printf("follows: could not prune old changes: %v", err)
			}
			continue
		case <-wake:
		case <-poll.C:
		}
		if _, err := Fanout(ctx, sqlDB); err != nil && ctx.Err() == nil {
			log.Printf("follows: fan-out failed, retrying: %v", err)
		}
	}
}

// Prune deletes changes handled before the given time that no unread notification still reports.
func Prune(ctx context.Context, sqlDB *sql.DB, before time.Time) (int64, error) {
	return db.New(sqlDB).DeleteOldSubjectChanges(ctx, sql.NullTime{Time: before, Valid: true})
}

// Fanout handles every pending change at once and returns how many it handled. In one snapshot it works out, for the
// persons followed, the clusters they joined and the cases newly linked to them; tells each follower of a changed
// subject, extending their unread notification for it; moves the follows of merged clusters; and marks the changes
// handled. Changes on subjects nobody follows are only marked.
func Fanout(ctx context.Context, sqlDB *sql.DB) (int, error) {
	tx, err := sqlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	locked, err := q.TryFanoutLock(ctx)
	if err != nil || !locked {
		return 0, err
	}
	pending, err := q.ListPendingChanges(ctx)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	follows, err := q.ListActiveFollows(ctx)
	if err != nil {
		return 0, err
	}
	followers := map[Subject][]int64{}
	var persons []string
	for _, f := range follows {
		s := Subject{Kind: f.SubjectKind, ID: f.SubjectID}
		if s.Kind == KindPerson && len(followers[s]) == 0 {
			persons = append(persons, s.ID)
		}
		followers[s] = append(followers[s], f.UserID)
	}

	type recorded struct {
		id int64
		Subject
	}
	all := make([]recorded, 0, len(pending))
	ids := make([]int64, 0, len(pending))
	for _, c := range pending {
		all = append(all, recorded{id: c.ID, Subject: Subject{Kind: c.SubjectKind, ID: c.SubjectID}})
		ids = append(ids, c.ID)
	}
	derived, err := derivePersonChanges(ctx, q, pending, persons)
	if err != nil {
		return 0, err
	}
	now := sql.NullTime{Time: time.Now(), Valid: true}
	for _, d := range derived {
		d.FannedOutAt = now
		id, err := q.InsertSubjectChange(ctx, d)
		if err != nil {
			return 0, err
		}
		all = append(all, recorded{id: id, Subject: Subject{Kind: d.SubjectKind, ID: d.SubjectID}})
	}

	type span struct{ first, last int64 }
	spans := map[Subject]*span{}
	var order []Subject
	for _, r := range all {
		if len(followers[r.Subject]) == 0 {
			continue
		}
		if sp, ok := spans[r.Subject]; ok {
			sp.first, sp.last = min(sp.first, r.id), max(sp.last, r.id)
			continue
		}
		spans[r.Subject] = &span{first: r.id, last: r.id}
		order = append(order, r.Subject)
	}
	for _, s := range order {
		kind := notifications.KindPersonChanged
		if s.Kind == KindCluster {
			kind = notifications.KindClusterChanged
		}
		sp := spans[s]
		for _, userID := range followers[s] {
			id, err := q.UpsertSubjectNotification(ctx, db.UpsertSubjectNotificationParams{
				UserID: userID, Kind: kind, Subject: s.ID,
				FirstChangeID: sql.NullInt64{Int64: sp.first, Valid: true},
				LastChangeID:  sql.NullInt64{Int64: sp.last, Valid: true},
			})
			if err != nil {
				return 0, err
			}
			if err := notifications.Signal(ctx, q, id); err != nil {
				return 0, err
			}
		}
	}
	for _, c := range pending {
		if c.Change == ChangeMerged && c.SubjectKind == KindCluster {
			if err := q.MoveClusterFollows(ctx, db.MoveClusterFollowsParams{FromCluster: c.SubjectID, ToCluster: c.Ref}); err != nil {
				return 0, err
			}
		}
	}
	if err := q.MarkChangesFannedOut(ctx, ids); err != nil {
		return 0, err
	}
	return len(pending), tx.Commit()
}

// batchEvent is the first membership change a batch holds for one feature: it tells where the feature stood before.
type batchEvent struct {
	added   bool
	cluster int64
}

// derivePersonChanges works out, for the persons followed, the AUTOMATIC clusters their KNOWN features joined and the
// cases newly linked to them, by comparing the clusters as they stand with how they stood before the pending changes.
// A case is linked when a QUESTIONED trace of it shares an AUTOMATIC cluster with one of the person's KNOWN features;
// comparing before with after, rather than looking at single members, keeps a merge or a member moving from reporting
// a case the person was already linked to.
func derivePersonChanges(ctx context.Context, q *db.Queries, pending []db.ListPendingChangesRow, persons []string) ([]db.InsertSubjectChangeParams, error) {
	if len(persons) == 0 {
		return nil, nil
	}
	first := map[string]batchEvent{}
	merged := map[int64]int64{}
	for _, c := range pending {
		if c.SubjectKind != KindCluster {
			continue
		}
		clusterID, err := strconv.ParseInt(c.SubjectID, 10, 64)
		if err != nil {
			continue
		}
		switch c.Change {
		case ChangeMemberAdded, ChangeMemberLeft:
			if _, seen := first[c.Ref]; !seen {
				first[c.Ref] = batchEvent{added: c.Change == ChangeMemberAdded, cluster: clusterID}
			}
		case ChangeMerged:
			if to, err := strconv.ParseInt(c.Ref, 10, 64); err == nil {
				merged[clusterID] = to
			}
		}
	}
	if len(first) == 0 {
		return nil, nil
	}

	knownRows, err := q.ListKnownFeaturesOfPersons(ctx, persons)
	if err != nil {
		return nil, err
	}
	known := map[string][]string{}
	var allKnown []string
	for _, r := range knownRows {
		known[r.PersonID] = append(known[r.PersonID], r.FeatureID)
		allKnown = append(allKnown, r.FeatureID)
	}
	if len(allKnown) == 0 {
		return nil, nil
	}
	current := map[string]int64{}
	memberships, err := q.ListClusterMembershipsForFeatureIDs(ctx, db.ListClusterMembershipsForFeatureIDsParams{Basis: basisAutomatic, FeatureIds: allKnown})
	if err != nil {
		return nil, err
	}
	for _, m := range memberships {
		current[m.FeatureID] = m.ClusterID
	}
	before := func(feature string) (int64, bool) {
		if e, ok := first[feature]; ok {
			return e.cluster, !e.added
		}
		c, ok := current[feature]
		return c, ok
	}

	nowClusters := map[string][]int64{}
	beforeClusters := map[string][]int64{}
	var clusterIDs []int64
	for _, p := range persons {
		for _, f := range known[p] {
			if c, ok := current[f]; ok && !slices.Contains(nowClusters[p], c) {
				nowClusters[p] = append(nowClusters[p], c)
				clusterIDs = append(clusterIDs, c)
			}
			if c, ok := before(f); ok && !slices.Contains(beforeClusters[p], c) {
				beforeClusters[p] = append(beforeClusters[p], c)
				clusterIDs = append(clusterIDs, c)
			}
		}
	}
	if len(clusterIDs) == 0 {
		return nil, nil
	}
	slices.Sort(clusterIDs)
	clusterIDs = slices.Compact(clusterIDs)
	memberRows, err := q.ListClusterMembersByClusterIDs(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}
	nowMembers := map[int64][]string{}
	beforeMembers := map[int64][]string{}
	for _, m := range memberRows {
		nowMembers[m.ClusterID] = append(nowMembers[m.ClusterID], m.FeatureID)
		if _, moved := first[m.FeatureID]; !moved {
			beforeMembers[m.ClusterID] = append(beforeMembers[m.ClusterID], m.FeatureID)
		}
	}
	for f, e := range first {
		if !e.added {
			beforeMembers[e.cluster] = append(beforeMembers[e.cluster], f)
		}
	}

	var traceIDs []int64
	for _, members := range []map[int64][]string{nowMembers, beforeMembers} {
		for _, fs := range members {
			for _, f := range fs {
				if id, ok := graph.ParseQuestionedFeatureID(f); ok {
					traceIDs = append(traceIDs, id)
				}
			}
		}
	}
	caseOf := map[int64]string{}
	if len(traceIDs) > 0 {
		rows, err := q.ListCasesByCaseTraceIDs(ctx, traceIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			caseOf[r.CaseTraceID] = r.CaseID
		}
	}
	casesIn := func(clusters []int64, members map[int64][]string) map[string]bool {
		out := map[string]bool{}
		for _, c := range clusters {
			for _, f := range members[c] {
				if id, ok := graph.ParseQuestionedFeatureID(f); ok && caseOf[id] != "" {
					out[caseOf[id]] = true
				}
			}
		}
		return out
	}
	survivor := func(c int64) int64 {
		for range 64 { // a chain of merges within one batch
			to, ok := merged[c]
			if !ok {
				break
			}
			c = to
		}
		return c
	}

	var out []db.InsertSubjectChangeParams
	for _, p := range persons {
		was := map[int64]bool{}
		for _, c := range beforeClusters[p] {
			was[survivor(c)] = true
		}
		joined := slices.Clone(nowClusters[p])
		slices.Sort(joined)
		for _, c := range joined {
			if !was[c] {
				out = append(out, db.InsertSubjectChangeParams{SubjectKind: KindPerson, SubjectID: p, Change: ChangeClusterJoined, Ref: strconv.FormatInt(c, 10), Amount: 1})
			}
		}
		linked := casesIn(beforeClusters[p], beforeMembers)
		var fresh []string
		for c := range casesIn(nowClusters[p], nowMembers) {
			if !linked[c] {
				fresh = append(fresh, c)
			}
		}
		slices.Sort(fresh)
		for _, c := range fresh {
			out = append(out, db.InsertSubjectChangeParams{SubjectKind: KindPerson, SubjectID: p, Change: ChangeCaseLinked, Ref: c, Amount: 1})
		}
	}
	return out, nil
}
