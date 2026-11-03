package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// Advisory lock keys (session-scoped), beside LockCaseDistribution's 7461001 and
// LockReportNumbering's 7461002. The keys are arbitrary.
const (
	// LockFaceSync and LockFingerprintSync let one sync per modality run at a time
	// (embedding.HandleSyncFace, fingerprint.HandleSync). asynq.Unique alone does not:
	// its lock lasts the debounce TTL, not the run, so once a sync outlives it every
	// new enqueue starts another, each redoing the same unmatched backlog. In the
	// stress test (docs/stress-test-plan.md) a 20x burst had eight fingerprint syncs
	// running at once, filling the worker and starving face embedding.
	LockFaceSync        int64 = 7461003
	LockFingerprintSync int64 = 7461004
	// lockClusterRun serialises Run's rewrite of the clusters, which both syncs end with.
	lockClusterRun int64 = 7461005
)

// TryExclusive runs fn while holding the advisory lock key, unless another session
// holds it: then it returns false without running fn.
func TryExclusive(ctx context.Context, sqlDB *sql.DB, key int64, fn func(context.Context) error) (bool, error) {
	return withLock(ctx, sqlDB, key, "SELECT pg_try_advisory_lock($1)", fn)
}

// exclusive runs fn while holding the advisory lock key, waiting for it first.
func exclusive(ctx context.Context, sqlDB *sql.DB, key int64, fn func(context.Context) error) error {
	_, err := withLock(ctx, sqlDB, key, "SELECT true FROM pg_advisory_lock($1)", fn)
	return err
}

// withLock takes a session lock on a connection of its own, held for fn's whole
// run while fn works on the pool's other connections.
func withLock(ctx context.Context, sqlDB *sql.DB, key int64, take string, fn func(context.Context) error) (bool, error) {
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("lock %d: %w", key, err)
	}
	defer conn.Close()
	var got bool
	if err := conn.QueryRowContext(ctx, take, key).Scan(&got); err != nil {
		return false, fmt.Errorf("lock %d: %w", key, err)
	}
	if !got {
		return false, nil
	}
	defer func() {
		// Unlock even when ctx was cancelled: the connection goes back to the pool,
		// and a session lock left on it would block every later sync.
		if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key); err != nil {
			log.Printf("cluster: unlock %d: %v", key, err)
		}
	}()
	return true, fn(ctx)
}
