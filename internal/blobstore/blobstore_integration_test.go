//go:build integration

package blobstore

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

// TestStore_RoundTrip exercises a real S3-compatible endpoint through the production
// minio-go path (Put → Get → Delete). It is skipped unless all four S3_* env vars are
// set, so it can run against a live bucket or a local MinIO without failing an offline
//
//	suite. Run: S3_ENDPOINT=… S3_BUCKET=… S3_ACCESS_KEY=… S3_SECRET_KEY=… \
//		go test -tags=integration ./internal/blobstore/
func TestStore_RoundTrip(t *testing.T) {
	store, err := New(Config{
		Endpoint:  os.Getenv("S3_ENDPOINT"),
		Bucket:    os.Getenv("S3_BUCKET"),
		AccessKey: os.Getenv("S3_ACCESS_KEY"),
		SecretKey: os.Getenv("S3_SECRET_KEY"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if store == nil {
		t.Skip("S3 not configured (set S3_ENDPOINT/S3_BUCKET/S3_ACCESS_KEY/S3_SECRET_KEY)")
	}

	ctx := context.Background()
	const key = "_it/blobstore-roundtrip.txt"
	want := []byte("freehire blobstore round-trip")
	t.Cleanup(func() { _ = store.Delete(ctx, key) })

	if err := store.Put(ctx, key, "text/plain", bytes.NewReader(want), int64(len(want))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip mismatch: got %q, want %q", got, want)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// minio Get is lazy — a missing object surfaces on read, not on the call.
	if rc, err := store.Get(ctx, key); err == nil {
		_, rerr := io.ReadAll(rc)
		rc.Close()
		if rerr == nil {
			t.Fatal("Get/read after Delete should fail")
		}
	}
}
