// Package storage wraps the MinIO/S3-compatible object store TrackID uses for file content that
// doesn't belong in Postgres or Memgraph (large binary payloads referenced by, but not embedded
// in, either domain model).
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// ErrNotFound is returned, wrapped, when a reference names an object or a file that isn't there.
var ErrNotFound = errors.New("storage: not found")

// Download fetches the content behind a storage reference. Three forms are read:
//   - "<bucket>/<objectKey>", as Upload returns, in this client's bucket;
//   - "s3://<bucket>/<objectKey>", an object in another bucket of the same endpoint;
//   - "file:///<absolute path>", a local file inside one of the roots WithFileRoots allowed.
func (client *Client) Download(ctx context.Context, storageRef string) ([]byte, error) {
	content, err := client.Open(ctx, storageRef)
	if err != nil {
		return nil, err
	}
	defer content.Close()
	return io.ReadAll(content)
}

// Open streams the content behind a storage reference, in the forms Download reads. A reference
// to nothing returns an error wrapping ErrNotFound.
func (client *Client) Open(ctx context.Context, storageRef string) (io.ReadCloser, error) {
	switch {
	case strings.HasPrefix(storageRef, "file://"):
		return client.openFile(strings.TrimPrefix(storageRef, "file://"))
	case strings.HasPrefix(storageRef, "s3://"):
		bucket, key, ok := strings.Cut(strings.TrimPrefix(storageRef, "s3://"), "/")
		if !ok || bucket == "" || key == "" {
			return nil, fmt.Errorf("storage ref %q is not s3://<bucket>/<key>", storageRef)
		}
		return client.open(ctx, bucket, key)
	}
	objectKey := strings.TrimPrefix(storageRef, client.bucket+"/")
	if client.bucket == "" || objectKey == storageRef {
		return nil, fmt.Errorf("storage ref %q does not belong to bucket %q", storageRef, client.bucket)
	}
	return client.open(ctx, client.bucket, objectKey)
}

// Digest reads the content behind a storage reference as a stream and returns its SHA-256, in
// lower-case hex as case_files.hash_id keeps it, and its size: a file of any size, without holding
// it in memory.
func (client *Client) Digest(ctx context.Context, storageRef string) (sha256Hex string, size int64, err error) {
	content, err := client.Open(ctx, storageRef)
	if err != nil {
		return "", 0, err
	}
	defer content.Close()
	hash := sha256.New()
	if size, err = io.Copy(hash, content); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func (client *Client) open(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	if client.minio == nil {
		return nil, fmt.Errorf("object storage is not configured")
	}
	object, err := client.minio.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject is lazy: Stat makes the request, so a missing object is known here.
	if _, err := object.Stat(); err != nil {
		object.Close()
		if code := minio.ToErrorResponse(err).Code; code == "NoSuchKey" || code == "NoSuchBucket" {
			return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, bucket, objectKey)
		}
		return nil, err
	}
	return object, nil
}

// openFile opens a local file, refusing any path that is not, once symlinks are resolved, inside
// one of the allowed roots.
func (client *Client) openFile(path string) (io.ReadCloser, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("storage: file reference %q is not an absolute path", path)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	if err != nil {
		return nil, err
	}
	for _, root := range client.fileRoots {
		if rel, err := filepath.Rel(root, resolved); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return os.Open(resolved)
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
