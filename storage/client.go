// Package storage wraps the MinIO/S3-compatible object store TrackID uses for file content that
// doesn't belong in Postgres or Memgraph (large binary payloads referenced by, but not embedded
// in, either domain model).
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	minio  *minio.Client
	bucket string
}

// NewClient connects to the configured MinIO/S3 endpoint and ensures the target bucket exists.
// endpoint may include a scheme (e.g. "http://localhost:59000"); TLS is used only for an
// explicit "https://" endpoint.
func NewClient(ctx context.Context, endpoint, accessKey, secretKey, bucket string) (*Client, error) {
	useSSL := strings.HasPrefix(endpoint, "https://")
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")

	minioClient, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	exists, err := minioClient.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("checking bucket %q: %w", bucket, err)
	}
	if !exists {
		if err := minioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("creating bucket %q: %w", bucket, err)
		}
	}
	return &Client{minio: minioClient, bucket: bucket}, nil
}

// Upload stores content under objectKey and returns an opaque "<bucket>/<objectKey>" reference,
// the same free-form storageRef shape the graph package's file linking already expects.
func (client *Client) Upload(ctx context.Context, objectKey string, content []byte, contentType string) (string, error) {
	_, err := client.minio.PutObject(ctx, client.bucket, objectKey, bytes.NewReader(content), int64(len(content)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", err
	}
	return client.bucket + "/" + objectKey, nil
}

// Download fetches the content behind a "<bucket>/<objectKey>" reference previously returned by
// Upload.
func (client *Client) Download(ctx context.Context, storageRef string) ([]byte, error) {
	objectKey := strings.TrimPrefix(storageRef, client.bucket+"/")
	if objectKey == storageRef {
		return nil, fmt.Errorf("storage ref %q does not belong to bucket %q", storageRef, client.bucket)
	}
	object, err := client.minio.GetObject(ctx, client.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	return io.ReadAll(object)
}
