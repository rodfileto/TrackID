package cluster

import (
	"context"
	"database/sql"
	"log"

	"github.com/rodfileto/trackid/db"
)

// MaxSettlePasses bounds Settle, so a feature that never leaves the unmatched backlog cannot keep
// a sync running forever; whatever is left waits for the next sync.
const MaxSettlePasses = 5

// Settle runs pass until nothing it reads changed while it ran: no human decision newer than the
// one before the pass, and no feature left unmatched (unmatched counts them). A sync task is
// enqueued with asynq.Unique, and asynq rejects a duplicate while one is queued or running --
// within the debounce TTL only; past it a new sync starts and finds the sync lock taken
// (LockFaceSync, LockFingerprintSync), so it leaves the work to this one and schedules a later
// sync. Either way an examiner's submit or a new embedding landing during a pass starts no other
// matching; without a further pass its decisions would wait for some later, unrelated sync to
// reach the clusters.
//
// What Settle cannot see is a change in the instant between its last check and asynq releasing
// the task's lock.
func Settle(ctx context.Context, sqlDB *sql.DB, unmatched func(context.Context) (int64, error), pass func(context.Context) error) error {
	q := db.New(sqlDB)
	for i := 1; ; i++ {
		before, err := q.LatestHumanDecisionID(ctx)
		if err != nil {
			return err
		}
		if err := pass(ctx); err != nil {
			return err
		}
		after, err := q.LatestHumanDecisionID(ctx)
		if err != nil {
			return err
		}
		left, err := unmatched(ctx)
		if err != nil {
			return err
		}
		if after == before && left == 0 {
			return nil
		}
		if i == MaxSettlePasses {
			log.Printf("cluster: still unsettled after %d passes (%d unmatched, decisions %d -> %d); leaving it to the next sync", i, left, before, after)
			return nil
		}
	}
}
