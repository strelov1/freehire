package blobstore

import "testing"

func TestResumeKey_DerivedFromUserID(t *testing.T) {
	if got := ResumeKey(7); got != "resumes/7" {
		t.Errorf("ResumeKey(7) = %q, want resumes/7", got)
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
	if PhotoKey(7) == ResumeKey(7) {
		t.Errorf("PhotoKey and ResumeKey collide at %q", PhotoKey(7))
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
