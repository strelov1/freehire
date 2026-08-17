package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
)

func TestContactHeaderEmpty(t *testing.T) {
	if !contactHeaderEmpty(cv.Header{}) {
		t.Fatal("empty header should report empty")
	}
	if contactHeaderEmpty(cv.Header{FullName: "Ada"}) {
		t.Fatal("name alone is not empty")
	}
}

func TestMergeSeedHeaderFillsGapsOnly(t *testing.T) {
	keep := cv.Header{FullName: "Keep", Email: ""}
	seeded := cv.Header{FullName: "Seed", Email: "new@example.com"}
	got := mergeSeedHeader(keep, seeded)
	if got.FullName != "Seed" {
		t.Errorf("FullName = %q, want Seed (non-empty seed replaces)", got.FullName)
	}
	if got.Email != "new@example.com" {
		t.Errorf("Email = %q, want filled", got.Email)
	}
	got2 := mergeSeedHeader(cv.Header{FullName: "Keep"}, cv.Header{FullName: ""})
	if got2.FullName != "Keep" {
		t.Errorf("empty seed must keep existing name, got %q", got2.FullName)
	}
}

func TestFillEmptyHeaderFieldsKeepsExistingName(t *testing.T) {
	keep := cv.Header{FullName: "Keep Me"}
	seed := cv.Header{FullName: "From Blob", Email: "blob@example.com"}
	got := fillEmptyHeaderFields(keep, seed)
	if got.FullName != "Keep Me" {
		t.Errorf("FullName = %q, want Keep Me", got.FullName)
	}
	if got.Email != "blob@example.com" {
		t.Errorf("Email = %q, want filled", got.Email)
	}
}

// A candidate whose owned overrides hold only a body field (no identity) must have
// resumeContactHeader fall through to the current extract rather than committing to
// owned's own (blank) identity subset. Regression: gating that fallthrough on
// owned.Empty() — now true for ANY owned field, not just identity — used to return a
// fully blank header with ok=false and never try the extract at all.
func TestResumeContactHeaderFallsThroughWhenOwnedIsBodyOnly(t *testing.T) {
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "Jane Doe", Email: "jane@example.com"})
	repo := &fakeResumeRepo{key: "resumes/1", set: true, structured: blob, structModel: "m",
		structAt: pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true}}
	store := resume.New(newFakeResumeBlobs(), repo)
	if _, err := store.SetCandidateOwned(context.Background(), 1, resume.Owned{Summary: "Staff engineer"}); err != nil {
		t.Fatal(err)
	}

	h := &cvHandlers{resume: store}
	hdr, ok, err := h.resumeContactHeader(context.Background(), 1)
	if err != nil {
		t.Fatalf("resumeContactHeader: %v", err)
	}
	if !ok || hdr.FullName != "Jane Doe" || hdr.Email != "jane@example.com" {
		t.Fatalf("header = %+v, ok:%v, want the extract's identity, not owned's blank one", hdr, ok)
	}
}
