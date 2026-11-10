package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrBucketNotLocked is returned by EnsureLockedBucket when the bucket exists without object lock,
// which can only be turned on when a bucket is created.
var ErrBucketNotLocked = errors.New("storage: the bucket exists without object lock")

// LockedBucket is what EnsureLockedBucket found or did.
type LockedBucket struct {
	Created bool // the bucket was made here
	Changed bool // its default retention was set or changed here
	Days    uint // the default retention now in force, in days
}

// EnsureLockedBucket makes sure bucket exists with S3 object lock and a default COMPLIANCE retention
// of days (docs/production-plan.md, decision 12): every object stored in it then can't be deleted
// or overwritten, by anyone, until its retention ends, and an overwrite only adds a version. A
// missing bucket is created locked. An existing one must already be locked (ErrBucketNotLocked
// otherwise), and its default retention is set to days when it differs; that applies to objects
// stored from then on, while each stored object keeps the retention it got.
func EnsureLockedBucket(ctx context.Context, endpoint, accessKey, secretKey, bucket string, days uint) (LockedBucket, error) {
	if days == 0 {
		return LockedBucket{}, errors.New("storage: the retention period must be at least one day")
	}
	client, err := minio.New(strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://"), &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: strings.HasPrefix(endpoint, "https://"),
	})
	if err != nil {
		return LockedBucket{}, err
	}

	var result LockedBucket
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return result, fmt.Errorf("checking bucket %q: %w", bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{ObjectLocking: true}); err != nil {
			return result, fmt.Errorf("creating bucket %q with object lock: %w", bucket, err)
		}
		result.Created = true
	}

	enabled, mode, validity, unit, err := client.GetObjectLockConfig(ctx, bucket)
	if err != nil || enabled != "Enabled" {
		if result.Created {
			return result, fmt.Errorf("bucket %q was created but reports no object lock: does the server support it? %v", bucket, err)
		}
		return result, fmt.Errorf("%w: %q", ErrBucketNotLocked, bucket)
	}
	if mode != nil && *mode == minio.Compliance && validity != nil && unit != nil && *unit == minio.Days && *validity == days {
		result.Days = days
		return result, nil
	}

	compliance, dayUnit := minio.Compliance, minio.Days
	if err := client.SetObjectLockConfig(ctx, bucket, &compliance, &days, &dayUnit); err != nil {
		return result, fmt.Errorf("setting bucket %q's default retention: %w", bucket, err)
	}
	result.Changed, result.Days = true, days
	return result, nil
}
