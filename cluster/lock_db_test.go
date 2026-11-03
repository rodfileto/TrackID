package cluster_test

import (
	"context"
	"testing"

	"github.com/rodfileto/trackid/cluster"
)

// While one session holds a sync lock, another does not run; once it is released, it does.
func TestTryExclusive(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()

	inner := false
	ran, err := cluster.TryExclusive(ctx, sqlDB, cluster.LockFingerprintSync, func(ctx context.Context) error {
		again, err := cluster.TryExclusive(ctx, sqlDB, cluster.LockFingerprintSync, func(context.Context) error {
			inner = true
			return nil
		})
		if err != nil {
			return err
		}
		if again {
			t.Error("a second sync ran while the first held the lock")
		}
		// Another modality's lock is independent.
		other, err := cluster.TryExclusive(ctx, sqlDB, cluster.LockFaceSync, func(context.Context) error { return nil })
		if err != nil {
			return err
		}
		if !other {
			t.Error("the face sync lock was taken by the fingerprint sync")
		}
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("first sync: ran %v, err %v", ran, err)
	}
	if inner {
		t.Fatal("the inner sync ran")
	}
	ran, err = cluster.TryExclusive(ctx, sqlDB, cluster.LockFingerprintSync, func(context.Context) error { return nil })
	if err != nil || !ran {
		t.Fatalf("after release: ran %v, err %v", ran, err)
	}
	// Run takes its own lock and releases it.
	if _, err := cluster.Run(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
	if _, err := cluster.Run(ctx, sqlDB); err != nil {
		t.Fatal(err)
	}
}
