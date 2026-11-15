// Package blobstore is a minimal, provider-agnostic object-storage abstraction.
// The application depends only on the Store interface and generic S3 settings
// (endpoint/bucket/access/secret) — no bucket name, host, or provider is baked in
// here; freehire-ops owns those via the environment. A nil Store means storage is
// unconfigured and the caller degrades.
package blobstore

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Store stores and retrieves blobs by key.
type Store interface {
	Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	// List returns the key of every object whose key starts with prefix.
	List(ctx context.Context, prefix string) ([]string, error)
}

// IsNotFound reports whether err, from Get or from reading the io.ReadCloser it returned,
// means the object does not exist, as opposed to a backend failure. Get is lazy, so a
// missing object can surface from either call site. It matches through %w wrapping.
func IsNotFound(err error) bool {
	var resp minio.ErrorResponse
	return errors.As(err, &resp) && resp.Code == minio.NoSuchKey
}

// Config is the generic S3 connection settings, read from the environment. Any
// S3-compatible endpoint works (Hetzner Object Storage, AWS, R2, MinIO).
type Config struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
}

func (c Config) configured() bool {
	return c.Endpoint != "" && c.Bucket != "" && c.AccessKey != "" && c.SecretKey != ""
}

// ResumeUploadKey is the object key one résumé upload is written under: the authenticated user
// id (never client input), the upload's instant and a random suffix, so each upload has its own
// object. Readers take the key from the pointer on the users row; old rows point at the fixed
// `resumes/<id>`. Parts are joined with "-", not "/": MinIO refuses an object under a prefix
// that is itself an object.
func ResumeUploadKey(userID int64, at time.Time) string {
	return fmt.Sprintf("%s%d-%s", ResumeUploadPrefix(userID), at.UnixNano(), rand.Text()[:16])
}

// ResumeUploadPrefix is what every upload key of one user starts with. The id ends at the "-",
// so user 1's prefix never matches user 12's keys.
func ResumeUploadPrefix(userID int64) string { return fmt.Sprintf("resumes/%d-", userID) }

// ResumeObjectKeys is every object a user's CV may be stored under, whatever the pointer says:
// each upload's object plus the legacy fixed key (named even if absent; deleting a missing
// object succeeds). Account deletion erases them all so no leftover outlives the account.
func ResumeObjectKeys(ctx context.Context, s Store, userID int64) ([]string, error) {
	keys, err := s.List(ctx, ResumeUploadPrefix(userID))
	if err != nil {
		return nil, err
	}
	return append(keys, fmt.Sprintf("resumes/%d", userID)), nil
}

// PhotoKey is the per-user object key for a stored headshot, derived from the
// authenticated user id, never from client input.
func PhotoKey(userID int64) string {
	return fmt.Sprintf("photos/%d", userID)
}

// New builds an S3-backed Store from the config, or returns (nil, nil) when the settings
// are incomplete (storage disabled). It does not dial.
func New(c Config) (Store, error) {
	if !c.configured() {
		return nil, nil
	}
	// minio wants the bare host plus a Secure flag, not a scheme.
	host := c.Endpoint
	secure := true
	if strings.Contains(host, "://") {
		u, err := url.Parse(c.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("blobstore: parse endpoint: %w", err)
		}
		host = u.Host
		secure = u.Scheme != "http"
	}
	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("blobstore: %w", err)
	}
	return &s3Store{client: client, bucket: c.Bucket}, nil
}

type s3Store struct {
	client *minio.Client
	bucket string
}

func (s *s3Store) Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("blobstore: put %s: %w", key, err)
	}
	return nil
}

func (s *s3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	// The request is lazy: a missing object surfaces as a read error.
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("blobstore: get %s: %w", key, err)
	}
	return obj, nil
}

func (s *s3Store) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("blobstore: delete %s: %w", key, err)
	}
	return nil
}

func (s *s3Store) List(ctx context.Context, prefix string) ([]string, error) {
	// Cancelling stops the listing goroutine on early return.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var keys []string
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("blobstore: list %s: %w", prefix, obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}
