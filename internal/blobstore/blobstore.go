// Package blobstore is a minimal, provider-agnostic object-storage abstraction.
// The application depends only on the Store interface and generic S3 settings
// (endpoint/bucket/access/secret) — no bucket name, host, or provider is baked in
// here; freehire-ops owns those via the environment. A nil Store means storage is
// unconfigured and the caller degrades.
package blobstore

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Store stores and retrieves blobs by key.
type Store interface {
	Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
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

// ResumeKey is the per-user object key for a stored résumé. It is derived from the
// authenticated user id, never from client input.
func ResumeKey(userID int64) string {
	return fmt.Sprintf("resumes/%d", userID)
}

// New builds an S3-backed Store from the config, or returns (nil, nil) when the
// settings are incomplete (storage disabled). minio.New does not dial here — the
// first network call happens on Put/Get/Delete.
func New(c Config) (Store, error) {
	if !c.configured() {
		return nil, nil
	}
	// The endpoint may include a scheme (https://host); minio wants the bare host
	// plus a Secure flag.
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
	// *minio.Object satisfies io.ReadCloser; the request is lazy — a missing object
	// surfaces as a read error, which the caller treats as "no résumé".
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
