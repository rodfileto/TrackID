// Package storage wraps the MinIO/S3-compatible object store TrackID uses for file content that
// doesn't belong in Postgres or Memgraph (large binary payloads referenced by, but not embedded
// in, either domain model).
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	minio  *minio.Client
	bucket string
	// fileRoots are the local directories a "file://" reference may point into (WithFileRoots).
	fileRoots []string
}

// FileRootsEnv names the environment variable listing, separated by the OS path list separator
// (":" on Linux), the local directories "file://" storage references may point into. Unset, no
// local file is served.
const FileRootsEnv = "STORAGE_FILE_ROOTS"

// FileRootsFromEnv reads FileRootsEnv.
func FileRootsFromEnv() []string {
	var roots []string
	for _, r := range filepath.SplitList(os.Getenv(FileRootsEnv)) {
		if r = strings.TrimSpace(r); r != "" {
			roots = append(roots, r)
		}
	}
	return roots
}

// WithFileRoots lets Download read "file://" references inside roots (and nowhere else) and
// returns client. An external original, such as an identity photo kept in the source register's
// own storage, is then referenced where it is instead of being copied into the bucket.
func (client *Client) WithFileRoots(roots []string) *Client {
	client.fileRoots = nil
	for _, r := range roots {
		if abs, err := filepath.Abs(r); err == nil {
			if resolved, err := filepath.EvalSymlinks(abs); err == nil {
				abs = resolved
			}
			client.fileRoots = append(client.fileRoots, abs)
		}
	}
	return client
}

// NewFileClient is a Client with no object store: it reads only "file://" references inside
// roots. Upload and bucket references fail.
func NewFileClient(roots []string) *Client {
	return (&Client{}).WithFileRoots(roots)
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
	// "file://" references are allowed where FileRootsEnv says, for every caller alike.
	return (&Client{minio: minioClient, bucket: bucket}).WithFileRoots(FileRootsFromEnv()), nil
}

// Upload stores content under objectKey and returns an opaque "<bucket>/<objectKey>" reference,
// the same free-form storageRef shape the graph package's file linking already expects.
func (client *Client) Upload(ctx context.Context, objectKey string, content []byte, contentType string) (string, error) {
	if client.minio == nil {
		return "", fmt.Errorf("object storage is not configured")
	}
	_, err := client.minio.PutObject(ctx, client.bucket, objectKey, bytes.NewReader(content), int64(len(content)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", err
	}
	return client.bucket + "/" + objectKey, nil
}

// Download fetches the content behind a storage reference. Three forms are read:
//   - "<bucket>/<objectKey>", as Upload returns, in this client's bucket;
//   - "s3://<bucket>/<objectKey>", an object in another bucket of the same endpoint;
//   - "file:///<absolute path>", a local file inside one of the roots WithFileRoots allowed.
func (client *Client) Download(ctx context.Context, storageRef string) ([]byte, error) {
	switch {
	case strings.HasPrefix(storageRef, "file://"):
		return client.readFile(strings.TrimPrefix(storageRef, "file://"))
	case strings.HasPrefix(storageRef, "s3://"):
		bucket, key, ok := strings.Cut(strings.TrimPrefix(storageRef, "s3://"), "/")
		if !ok || bucket == "" || key == "" {
			return nil, fmt.Errorf("storage ref %q is not s3://<bucket>/<key>", storageRef)
		}
		return client.get(ctx, bucket, key)
	}
	objectKey := strings.TrimPrefix(storageRef, client.bucket+"/")
	if client.bucket == "" || objectKey == storageRef {
		return nil, fmt.Errorf("storage ref %q does not belong to bucket %q", storageRef, client.bucket)
	}
	return client.get(ctx, client.bucket, objectKey)
}

func (client *Client) get(ctx context.Context, bucket, objectKey string) ([]byte, error) {
	if client.minio == nil {
		return nil, fmt.Errorf("object storage is not configured")
	}
	object, err := client.minio.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	return io.ReadAll(object)
}

// readFile reads a local file, refusing any path that is not, once symlinks are resolved, inside
// one of the allowed roots.
func (client *Client) readFile(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("storage: file reference %q is not an absolute path", path)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	for _, root := range client.fileRoots {
		if rel, err := filepath.Rel(root, resolved); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return os.ReadFile(resolved)
		}
	}
	return nil, fmt.Errorf("storage: file %q is outside the allowed roots (%s)", path, FileRootsEnv)
}

// Ping checks that the object storage answers and the client's bucket exists.
func (client *Client) Ping(ctx context.Context) error {
	if client == nil || client.minio == nil {
		return fmt.Errorf("object storage is not configured")
	}
	exists, err := client.minio.BucketExists(ctx, client.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("bucket %q does not exist", client.bucket)
	}
	return nil
}
