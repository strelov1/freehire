package resume

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/resumeextract"
)

func TestFillEmptyDoesNotOverwriteOwned(t *testing.T) {
	dst := Contacts{Email: "mine@example.com", Links: []string{"https://mine.example"}}
	src := Contacts{FullName: "Ada", Email: "other@example.com", Links: []string{"https://other.example"}}
	FillEmpty(&dst, src)
	if dst.Email != "mine@example.com" || len(dst.Links) != 1 || dst.Links[0] != "https://mine.example" {
		t.Fatalf("owned fields overwritten: %+v", dst)
	}
	if dst.FullName != "Ada" {
		t.Fatalf("empty name not filled: %+v", dst)
	}
}

func TestSetStructuredFillsEmptyContactsOnly(t *testing.T) {
	repo := newFakeRepo()
	s := New(nil, repo)
	t1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	if _, err := s.SetCandidateContacts(context.Background(), 7, Contacts{
		Email: "keep@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStructured(context.Background(), 7, resumeextract.Structured{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Links:    []string{"https://ada.example"},
	}, "m", t1); err != nil {
		t.Fatal(err)
	}
	got, err := s.CandidateContacts(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "keep@example.com" {
		t.Fatalf("email = %q, want kept", got.Email)
	}
	if got.FullName != "Ada Lovelace" || len(got.Links) != 1 {
		t.Fatalf("empty fields not filled: %+v", got)
	}
}

// A slow/late extraction for a since-superseded upload must not leak into candidate-owned
// contacts. repo.SetStructured's monotonic stamp guard already drops the structured/
// geography write when the stamp is stale; Store.SetStructured must skip the contacts
// fill in that same case rather than filling from data the write itself discarded.
func TestSetStructuredWithStaleStampDoesNotFillContacts(t *testing.T) {
	repo := newFakeRepo()
	s := New(nil, repo)
	ctx := context.Background()

	t1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	// A newer upload (t2) has already replaced the CV extraction A was derived from (t1).
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t2, Valid: true}

	stale := resumeextract.Structured{FullName: "Old Name", Phone: "111-111-1111"}
	if err := s.SetStructured(ctx, 7, stale, "model-x", t1); err != nil {
		t.Fatalf("SetStructured: %v", err)
	}

	if len(repo.structured[7]) != 0 {
		t.Fatalf("structured blob = %q, want the guard to have dropped the write", repo.structured[7])
	}
	got, err := s.CandidateContacts(ctx, 7)
	if err != nil {
		t.Fatalf("CandidateContacts: %v", err)
	}
	if !got.Empty() {
		t.Fatalf("contacts = %+v, want empty — a dropped stale write must not leak into owned contacts", got)
	}
}

func TestClearKeepsCandidateContacts(t *testing.T) {
	repo := newFakeRepo()
	blobs := &fakeBlobs{objs: map[string][]byte{}}
	s := New(blobs, repo)
	if _, err := s.Put(context.Background(), 7, "text/plain", []byte("cv")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetCandidateContacts(context.Background(), 7, Contacts{FullName: "Ada"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	got, err := s.CandidateContacts(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.FullName != "Ada" {
		t.Fatalf("contacts cleared on résumé delete: %+v", got)
	}
}

func TestStructureForSeedOwnedOverlayOnCurrentExtract(t *testing.T) {
	repo := newFakeRepo()
	s := New(nil, repo)
	t1 := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "From Blob",
		Email:    "blob@example.com",
		Summary:  "Staff engineer",
	})
	repo.structured[7] = blob
	repo.structAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	if _, err := s.SetCandidateContacts(context.Background(), 7, Contacts{
		FullName: "Ada Lovelace", Email: "ada@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	st, ok, err := s.StructureForSeed(context.Background(), 7)
	if err != nil || !ok {
		t.Fatalf("StructureForSeed = ok:%v err:%v", ok, err)
	}
	if st.FullName != "Ada Lovelace" || st.Email != "ada@example.com" {
		t.Fatalf("owned overlay missing: %+v", st)
	}
	if st.Summary != "Staff engineer" {
		t.Fatalf("current body dropped: %+v", st)
	}
}

func TestStructureForSeedPendingBlobIsContactsOnly(t *testing.T) {
	repo := newFakeRepo()
	s := New(nil, repo)
	tOld := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	tNew := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: tNew, Valid: true}
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Old",
		Summary:  "Ten years of systems work.",
		Skills:   []string{"Go", "Kafka"},
		Projects: []resumeextract.Project{{Name: "opensched"}},
	})
	repo.structured[7] = blob
	repo.structAt[7] = pgtype.Timestamptz{Time: tOld, Valid: true}
	if _, err := s.SetCandidateContacts(context.Background(), 7, Contacts{FullName: "Ada", Links: []string{"https://ada.example"}}); err != nil {
		t.Fatal(err)
	}
	st, ok, err := s.StructureForSeed(context.Background(), 7)
	if err != nil || !ok {
		t.Fatalf("StructureForSeed = ok:%v err:%v", ok, err)
	}
	if st.FullName != "Ada" || len(st.Links) != 1 {
		t.Fatalf("contacts = %+v", st)
	}
	if st.Summary != "" || len(st.Skills) != 0 || len(st.Projects) != 0 {
		t.Fatalf("pending seed leaked superseded semantics: %+v", st)
	}
}
