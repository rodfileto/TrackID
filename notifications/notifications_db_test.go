package notifications_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/notifications"
	"github.com/rodfileto/trackid/realtime"
)

// testDB is a throwaway, migrated database on the server named by TRACKID_TEST_DATABASE_URL,
// skipped when unset. It also returns the database's URL, for the listener's own connection.
func testDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	adminURL := os.Getenv("TRACKID_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TRACKID_TEST_DATABASE_URL not set")
	}
	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	name := fmt.Sprintf("trackid_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`) })
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	sqlDB, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	return sqlDB, u.String()
}

func newUser(t *testing.T, sqlDB *sql.DB, name string) auth.UserProfile {
	t.Helper()
	user, err := auth.NewRepository(sqlDB).Create(context.Background(), auth.NewUser{FirstName: name, LastName: "Test", StaffNumber: name,
		JobTitle: "test", Username: name, Password: "password-1", Access: auth.Access{AccountRole: auth.AccountRoleUser, IsActive: true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	return user
}

type recorder struct {
	mu     sync.Mutex
	topics []string
}

func (r *recorder) Publish(topic string, _ realtime.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.topics = append(r.topics, topic)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.topics)
}

func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func listeners(t *testing.T, sqlDB *sql.DB) int {
	t.Helper()
	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM pg_stat_activity
		WHERE datname = current_database() AND query LIKE 'LISTEN%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func createIn(t *testing.T, sqlDB *sql.DB, userID int64, finish func(*sql.Tx) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := notifications.Create(ctx, db.New(tx), userID, notifications.KindAnalysisAssigned, "1"); err != nil {
		t.Fatal(err)
	}
	if err := finish(tx); err != nil {
		t.Fatal(err)
	}
}

// The signal follows the transaction: nothing for a rollback, and one signal, to the right user,
// for a commit. When the listener's connection dies it comes back and asks everyone to resync.
func TestSignalFollowsTheTransaction(t *testing.T) {
	sqlDB, databaseURL := testDB(t)
	user := newUser(t, sqlDB, "ana")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	publisher := &recorder{}
	var mu sync.Mutex
	reconnections := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		realtime.Listen(ctx, databaseURL, notifications.Channel, notifications.Relay(sqlDB, publisher), func() {
			mu.Lock()
			defer mu.Unlock()
			reconnections++
		})
	}()
	eventually(t, "the listener", func() bool { return listeners(t, sqlDB) == 1 })

	createIn(t, sqlDB, user.ID, (*sql.Tx).Rollback)
	createIn(t, sqlDB, user.ID, (*sql.Tx).Commit)
	eventually(t, "the signal", func() bool { return publisher.count() >= 1 })
	time.Sleep(100 * time.Millisecond)
	if publisher.count() != 1 || publisher.topics[0] != realtime.UserTopic(user.ID) {
		t.Fatalf("published to %v", publisher.topics)
	}

	if _, err := sqlDB.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
		WHERE datname = current_database() AND query LIKE 'LISTEN%'`); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the reconnection", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reconnections == 1
	})
	createIn(t, sqlDB, user.ID, (*sql.Tx).Commit)
	eventually(t, "a signal after reconnecting", func() bool { return publisher.count() == 2 })

	cancel()
	<-done
}

func TestReadStateBelongsToItsUser(t *testing.T) {
	ctx := context.Background()
	sqlDB, _ := testDB(t)
	ana, bia := newUser(t, sqlDB, "ana"), newUser(t, sqlDB, "bia")
	createIn(t, sqlDB, ana.ID, (*sql.Tx).Commit)
	createIn(t, sqlDB, ana.ID, (*sql.Tx).Commit)
	createIn(t, sqlDB, bia.ID, (*sql.Tx).Commit)

	items, unread, err := notifications.List(ctx, sqlDB, ana.ID)
	if err != nil || len(items) != 2 || unread != 2 {
		t.Fatalf("list = %+v, %d, %v", items, unread, err)
	}
	if err := notifications.MarkRead(ctx, sqlDB, items[0].ID, bia.ID); err != nil {
		t.Fatal(err)
	}
	if _, unread, _ := notifications.List(ctx, sqlDB, ana.ID); unread != 2 {
		t.Fatalf("another user changed ana's notification: unread = %d", unread)
	}
	if err := notifications.MarkRead(ctx, sqlDB, items[0].ID, ana.ID); err != nil {
		t.Fatal(err)
	}
	if _, unread, _ := notifications.List(ctx, sqlDB, ana.ID); unread != 1 {
		t.Fatalf("unread = %d, want 1", unread)
	}
	if err := notifications.MarkAllRead(ctx, sqlDB, ana.ID); err != nil {
		t.Fatal(err)
	}
	if _, unread, _ := notifications.List(ctx, sqlDB, ana.ID); unread != 0 {
		t.Fatalf("unread = %d, want 0", unread)
	}
	if _, unread, _ := notifications.List(ctx, sqlDB, bia.ID); unread != 1 {
		t.Fatalf("bia's unread = %d, want 1", unread)
	}
}
