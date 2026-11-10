package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// The lock tests need an S3 server with object lock, such as the dev stack's SeaweedFS:
// TRACKID_TEST_S3_ENDPOINT=http://localhost:59000 TRACKID_TEST_S3_ACCESS_KEY=... TRACKID_TEST_S3_SECRET_KEY=...
// Each run makes buckets of its own, which stay: their objects are locked.
func testS3(t *testing.T) (endpoint, accessKey, secretKey string, client *minio.Client) {
	t.Helper()
	endpoint = os.Getenv("TRACKID_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TRACKID_TEST_S3_ENDPOINT is not set")
	}
	accessKey, secretKey = os.Getenv("TRACKID_TEST_S3_ACCESS_KEY"), os.Getenv("TRACKID_TEST_S3_SECRET_KEY")
	client, err := minio.New(endpoint[len("http://"):], &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, "")})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint, accessKey, secretKey, client
}

func TestEnsureLockedBucketCreatesALockedBucket(t *testing.T) {
	endpoint, accessKey, secretKey, raw := testS3(t)
	ctx := context.Background()
	bucket := fmt.Sprintf("trackid-lock-test-%d", time.Now().UnixNano())

	got, err := EnsureLockedBucket(ctx, endpoint, accessKey, secretKey, bucket, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Created || !got.Changed || got.Days != 1 {
		t.Fatalf("first run: %+v", got)
	}

	// Core's client works on it unchanged, and what it stores can't be deleted.
	client, err := NewClient(ctx, endpoint, accessKey, secretKey, bucket)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Upload(ctx, "evidence/a.jpg", []byte("a"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	info, err := raw.StatObject(ctx, bucket, "evidence/a.jpg", minio.StatObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.RemoveObject(ctx, bucket, "evidence/a.jpg", minio.RemoveObjectOptions{VersionID: info.VersionID}); err == nil {
		t.Fatal("deleting a stored version succeeded; it should be locked")
	}

	// Again, nothing to do; with another period, the default changes.
	if got, err := EnsureLockedBucket(ctx, endpoint, accessKey, secretKey, bucket, 1); err != nil || got.Created || got.Changed {
		t.Fatalf("second run: %+v %v", got, err)
	}
	if got, err := EnsureLockedBucket(ctx, endpoint, accessKey, secretKey, bucket, 2); err != nil || !got.Changed || got.Days != 2 {
		t.Fatalf("new period: %+v %v", got, err)
	}
}

func TestEnsureLockedBucketRefusesAnUnlockedBucket(t *testing.T) {
	endpoint, accessKey, secretKey, raw := testS3(t)
	ctx := context.Background()
	bucket := fmt.Sprintf("trackid-unlocked-test-%d", time.Now().UnixNano())
	if err := raw.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	defer raw.RemoveBucket(ctx, bucket)

	if _, err := EnsureLockedBucket(ctx, endpoint, accessKey, secretKey, bucket, 1); !errors.Is(err, ErrBucketNotLocked) {
		t.Fatalf("expected ErrBucketNotLocked, got %v", err)
	}
}
