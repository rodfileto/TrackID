package cluster_test

import (
	"context"
	"slices"
	"testing"
	"time"

	. "github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/identity"
)

// A withdrawn register's photo stops matching: its embedding goes, the AUTOMATIC layer drops the link only the
// matcher made to it, and the links people confirmed stay in both layers. Restoring brings the matcher's link back.
func TestWithdrawnFeatureLeavesSystemLinksOnly(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	w := &conflictWorld{t: t, db: sqlDB}
	for n := int64(1); n <= 3; n++ {
		w.person(n)
	}
	w.record(1, "Ana Silva", 1, 2) // register 1
	w.record(2, "Ana Silva", 3, 4) // register 2
	w.record(3, "Ana Silva", 5, 6) // register 3
	w.embed(1, 0)
	w.embed(3, 0)
	w.embed(5, 0)
	w.matcher("1", "3", "FACIAL", 0.9, 0.7) // the matcher alone
	w.examiners("1", "5", "FACIAL", "POSITIVE")

	members := func(basis string) [][]string {
		t.Helper()
		rows, err := sqlDB.Query(`SELECT cluster_id, feature_id FROM cluster_members WHERE basis = $1 ORDER BY cluster_id, feature_id`, basis)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		byCluster := map[int64][]string{}
		var order []int64
		for rows.Next() {
			var id int64
			var feature string
			if err := rows.Scan(&id, &feature); err != nil {
				t.Fatal(err)
			}
			if _, ok := byCluster[id]; !ok {
				order = append(order, id)
			}
			byCluster[id] = append(byCluster[id], feature)
		}
		var out [][]string
		for _, id := range order {
			if len(byCluster[id]) > 1 {
				out = append(out, byCluster[id])
			}
		}
		return out
	}
	run := func() {
		t.Helper()
		if _, err := Run(ctx, sqlDB); err != nil {
			t.Fatal(err)
		}
	}
	same := func(name string, got [][]string, want ...[]string) {
		t.Helper()
		if !slices.EqualFunc(got, want, slices.Equal[[]string]) {
			t.Fatalf("%s: %v, want %v", name, got, want)
		}
	}

	run()
	same("automatic", members("AUTOMATIC"), []string{"1", "3", "5"})
	same("reviewed", members("REVIEWED"), []string{"1", "5"})

	withdrawn, err := identity.WithdrawRegister(ctx, sqlDB, 1, "analyst", "not the person", time.Now(), nil)
	if err != nil || withdrawn != 1 {
		t.Fatalf("withdrew %d, %v", withdrawn, err)
	}
	var embeddings int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM feature_embeddings e JOIN biometric_features f ON f.id = e.biometric_feature_id WHERE f.identity_file_id = 1`).Scan(&embeddings); err != nil || embeddings != 0 {
		t.Fatalf("%d embeddings left, %v", embeddings, err)
	}
	if again, err := identity.WithdrawRegister(ctx, sqlDB, 1, "analyst", "again", time.Now(), nil); err != nil || again != 0 {
		t.Fatalf("withdrew again: %d, %v", again, err)
	}
	if _, err := identity.WithdrawRegister(ctx, sqlDB, 1, "", "", time.Now(), nil); err == nil {
		t.Fatal("withdrawn without who and why")
	}
	run()
	same("automatic, withdrawn", members("AUTOMATIC"), []string{"1", "5"})
	same("reviewed, withdrawn", members("REVIEWED"), []string{"1", "5"})

	restored, err := identity.RestoreRegister(ctx, sqlDB, 1, nil)
	if err != nil || restored != 1 {
		t.Fatalf("restored %d, %v", restored, err)
	}
	run()
	same("automatic, restored", members("AUTOMATIC"), []string{"1", "3", "5"})
}
