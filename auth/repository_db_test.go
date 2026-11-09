package auth_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"golang.org/x/crypto/bcrypt"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db/migrations"
)

// testDB mirrors requests/requests_db_test.go's helper: a throwaway, migrated database on the
// server named by TRACKID_TEST_DATABASE_URL, skipped when unset.
func testDB(t *testing.T) *sql.DB {
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
	return sqlDB
}

// A user changes their own name and job title, and their password given the current one; the
// username, staff number and email stay.
func TestSelfService(t *testing.T) {
	ctx := context.Background()
	repo := auth.NewRepository(testDB(t))
	user, err := repo.Create(ctx, auth.NewUser{FirstName: "Ana", LastName: "Silva", StaffNumber: "123", JobTitle: "Perita",
		Username: "ana", Password: "first-password", Access: auth.Access{AccountRole: auth.AccountRoleUser, IsActive: true}}, "example.org")
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.UpdateDetails(ctx, user.ID, auth.Details{FirstName: " Ana Paula ", LastName: "Silva Souza", JobTitle: "Perita Criminal"})
	if err != nil {
		t.Fatal(err)
	}
	if got.FirstName != "Ana Paula" || got.LastName != "Silva Souza" || got.JobTitle != "Perita Criminal" {
		t.Errorf("details %+v", got)
	}
	if got.Username != "ana" || got.StaffNumber != "123" || got.Email != "ana@example.org" {
		t.Errorf("fixed fields changed: %+v", got)
	}
	if _, err := repo.UpdateDetails(ctx, user.ID, auth.Details{FirstName: " ", LastName: "Silva"}); !errors.Is(err, auth.ErrInvalidDetails) {
		t.Errorf("empty first name: %v, want ErrInvalidDetails", err)
	}

	if err := repo.ChangePassword(ctx, user.ID, "not-it", "second-password"); !errors.Is(err, auth.ErrWrongPassword) {
		t.Errorf("wrong current password: %v, want ErrWrongPassword", err)
	}
	if err := repo.ChangePassword(ctx, user.ID, "first-password", "short"); !errors.Is(err, auth.ErrPasswordTooShort) {
		t.Errorf("short new password: %v, want ErrPasswordTooShort", err)
	}
	if err := repo.ChangePassword(ctx, user.ID, "first-password", "second-password"); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.FindByStaffNumber(ctx, "123")
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("second-password")) != nil {
		t.Error("the new password does not sign in")
	}
}

// A picture of any shape is stored as the central square at AvatarSize, and can be removed.
func TestAvatar(t *testing.T) {
	ctx := context.Background()
	repo := auth.NewRepository(testDB(t))
	user, err := repo.Create(ctx, auth.NewUser{FirstName: "Ana", LastName: "Silva", StaffNumber: "123", Username: "ana",
		Password: "first-password", Access: auth.Access{AccountRole: auth.AccountRoleUser, IsActive: true}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if user.AvatarUpdatedAt != nil {
		t.Fatal("a new user has a picture")
	}
	if _, err := repo.SetAvatar(ctx, user.ID, []byte("not an image")); !errors.Is(err, auth.ErrNotAnImage) {
		t.Errorf("text upload: %v, want ErrNotAnImage", err)
	}
	wide := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for x := 0; x < 400; x++ {
		for y := 0; y < 200; y++ {
			wide.Set(x, y, color.RGBA{uint8(x / 2), 80, 160, 255})
		}
	}
	var upload bytes.Buffer
	if err := png.Encode(&upload, wide); err != nil {
		t.Fatal(err)
	}
	profile, err := repo.SetAvatar(ctx, user.ID, upload.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if profile.AvatarUpdatedAt == nil {
		t.Error("the profile does not show the picture")
	}
	data, _, err := repo.Avatar(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("stored picture is not a JPEG: %v", err)
	}
	if b := stored.Bounds(); b.Dx() != auth.AvatarSize || b.Dy() != auth.AvatarSize {
		t.Errorf("stored %v, want %d square", b, auth.AvatarSize)
	}
	if profile, err = repo.RemoveAvatar(ctx, user.ID); err != nil || profile.AvatarUpdatedAt != nil {
		t.Errorf("remove: %v, %+v", err, profile.AvatarUpdatedAt)
	}
	if _, _, err := repo.Avatar(ctx, user.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("after removal: %v, want sql.ErrNoRows", err)
	}
}
