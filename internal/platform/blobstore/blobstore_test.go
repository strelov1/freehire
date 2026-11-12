package blobstore

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

func TestResumeUploadKey_DerivedFromUserIDAndInstant(t *testing.T) {
	at := time.Unix(1_700_000_000, 5)
	if got := ResumeUploadKey(7, at); got != "resumes/7-1700000000000000005" {
		t.Errorf("ResumeUploadKey = %q, want resumes/7-1700000000000000005", got)
	}
	if ResumeUploadKey(7, at) == ResumeUploadKey(7, at.Add(time.Nanosecond)) {
		t.Error("two uploads share one key")
	}
}

func TestPhotoKey_DerivedFromUserID(t *testing.T) {
	if got := PhotoKey(7); got != "photos/7" {
		t.Errorf("PhotoKey(7) = %q, want photos/7", got)
	}
}

func TestPhotoKey_DoesNotCollideWithResumeKey(t *testing.T) {
	// The two per-user objects share a bucket, so their prefixes must differ — a
	// collision would have an upload silently overwrite the stored CV.
	if key := ResumeUploadKey(7, time.Unix(0, 0)); strings.HasPrefix(key, "photos/") {
		t.Errorf("a résumé key %q shares the photos prefix", key)
	}
}

func TestNew_UnconfiguredReturnsNilStore(t *testing.T) {
	cases := []Config{
		{},
		{Endpoint: "https://hel1.example.com"}, // missing the rest
		{Endpoint: "https://hel1.example.com", Bucket: "b"},                  // missing keys
		{Endpoint: "https://hel1.example.com", Bucket: "b", AccessKey: "ak"}, // missing secret
	}
	for i, c := range cases {
		store, err := New(c)
		if err != nil {
			t.Fatalf("case %d: New returned error for unconfigured: %v", i, err)
		}
		if store != nil {
			t.Errorf("case %d: unconfigured New should return nil store, got %T", i, store)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"NoSuchKey", minio.ErrorResponse{Code: minio.NoSuchKey}, true},
		{
			"NoSuchKey wrapped by Get/Put/Delete's %w",
			fmt.Errorf("blobstore: get resumes/7: %w", minio.ErrorResponse{Code: minio.NoSuchKey}),
			true,
		},
		{"a different S3 error code", minio.ErrorResponse{Code: "AccessDenied"}, false},
		{"a non-minio error", errors.New("connection reset"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNotFound(tc.err); got != tc.want {
				t.Errorf("IsNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestNew_ConfiguredReturnsStore(t *testing.T) {
	// minio.New does not dial on construction, so a fully-configured Config yields a
	// usable Store without any network.
	store, err := New(Config{
		Endpoint:  "https://hel1.your-objectstorage.com",
		Bucket:    "freehire-resumes",
		AccessKey: "ak",
		SecretKey: "sk",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if store == nil {
		t.Fatal("configured New should return a non-nil store")
	}
}
