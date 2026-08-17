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
	dst := Owned{Email: "mine@example.com", Links: []string{"https://mine.example"}}
	src := Owned{FullName: "Ada", Email: "other@example.com", Links: []string{"https://other.example"}}
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
	if _, err := s.SetCandidateOwned(context.Background(), 7, Owned{
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
	got, err := s.CandidateOwned(context.Background(), 7)
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
	got, err := s.CandidateOwned(ctx, 7)
	if err != nil {
		t.Fatalf("CandidateOwned: %v", err)
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
	if _, err := s.SetCandidateOwned(context.Background(), 7, Owned{FullName: "Ada"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	got, err := s.CandidateOwned(context.Background(), 7)
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
	if _, err := s.SetCandidateOwned(context.Background(), 7, Owned{
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

// A candidate who has only ever edited a body field (e.g. Summary via CvSummaryCard) has
// an Owned whose identity fields are still blank — Owned itself is non-Empty (Summary is
// set), but that must not gate a block-overwrite of a real name/email pulled from the
// current extract. Regression for the Owned.Empty()-vs-IdentityEmpty() bug: widening
// Empty() to also cover the new body fields once made this identical to gating on
// IdentityEmpty(), which silently blanked identity for anyone who saved a body field
// before ever touching Contacts.
func TestStructureForSeedBodyOnlyOwnedDoesNotBlankIdentity(t *testing.T) {
	repo := newFakeRepo()
	s := New(nil, repo)
	t1 := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	repo.uploadedAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "From Blob",
		Email:    "blob@example.com",
	})
	repo.structured[7] = blob
	repo.structAt[7] = pgtype.Timestamptz{Time: t1, Valid: true}
	if _, err := s.SetCandidateOwned(context.Background(), 7, Owned{Summary: "Staff engineer"}); err != nil {
		t.Fatal(err)
	}
	st, ok, err := s.StructureForSeed(context.Background(), 7)
	if err != nil || !ok {
		t.Fatalf("StructureForSeed = ok:%v err:%v", ok, err)
	}
	if st.FullName != "From Blob" || st.Email != "blob@example.com" {
		t.Fatalf("body-only owned blanked identity: %+v", st)
	}
	if st.Summary != "Staff engineer" {
		t.Fatalf("owned summary not applied: %+v", st)
	}
}

func TestOwnedEmptyVsIdentityEmpty(t *testing.T) {
	bodyOnly := Owned{Summary: "hi"}
	if bodyOnly.Empty() {
		t.Fatal("Empty() = true for an owned block with a body field set")
	}
	if !bodyOnly.IdentityEmpty() {
		t.Fatal("IdentityEmpty() = false for an owned block with no identity fields set")
	}

	identityOnly := Owned{FullName: "Ada"}
	if identityOnly.IdentityEmpty() {
		t.Fatal("IdentityEmpty() = true for an owned block with FullName set")
	}
}

// A candidate who deliberately empties a body field they had previously set (e.g. clears
// Headline in CvSummaryCard) must see that clear stick, not the field silently reappear
// from whatever the CV extract says. Before the *Set flags, ApplyBody's own non-empty
// check made "" indistinguishable from "never touched" — this is the regression test for
// that bug (freehire#2026 review).
func TestApplyBody_OwnedEmptyOverridesExtract(t *testing.T) {
	st := resumeextract.Structured{Headline: "From CV", Summary: "From CV summary", Languages: []string{"French"}}
	owned := Owned{HeadlineSet: true, SummarySet: true, LanguagesSet: true} // cleared, not untouched
	owned.ApplyBody(&st)

	if st.Headline != "" || st.Summary != "" || len(st.Languages) != 0 {
		t.Fatalf("an owned-empty field fell back to the extract: %+v", st)
	}
}

// The counterpart: a field the candidate never touched at all (Set false, value "") must
// still pass the extract's own value through untouched — ApplyBody's fix must not turn
// every unedited field into a forced blank.
func TestApplyBody_UntouchedFieldPassesExtractThrough(t *testing.T) {
	st := resumeextract.Structured{Headline: "From CV", Education: []resumeextract.Education{{Degree: "BSc"}}}
	Owned{}.ApplyBody(&st)

	if st.Headline != "From CV" || len(st.Education) != 1 {
		t.Fatalf("an untouched field was blanked instead of passed through: %+v", st)
	}
}

// A fresh CV upload must not resurrect a field the candidate explicitly cleared — the same
// protection FillEmpty already gave a non-empty edit, extended to an owned-empty one.
func TestFillEmptyDoesNotRefillAnExplicitClear(t *testing.T) {
	dst := Owned{HeadlineSet: true} // cleared: Headline is "" on purpose
	src := Owned{Headline: "Newly extracted headline"}
	FillEmpty(&dst, src)

	if dst.Headline != "" {
		t.Fatalf("an explicit clear was refilled from a fresh extract: %+v", dst)
	}
}

// Round-tripped through the store exactly as PutResumeContacts does: Sanitize runs on
// write (SetCandidateOwned) and again on read (decodeOwned via CandidateOwned), and an
// explicit clear must survive both without a non-empty value to re-derive Set from.
func TestSetCandidateOwned_ExplicitClearSurvivesTheStore(t *testing.T) {
	repo := newFakeRepo()
	s := New(nil, repo)
	if _, err := s.SetCandidateOwned(context.Background(), 7, Owned{
		Headline: "Staff Engineer", HeadlineSet: true,
	}); err != nil {
		t.Fatal(err)
	}
	// The candidate reopens the card and clears it.
	if _, err := s.SetCandidateOwned(context.Background(), 7, Owned{
		Headline: "", HeadlineSet: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.CandidateOwned(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.Headline != "" || !got.HeadlineSet {
		t.Fatalf("clear did not survive a write+read round trip: %+v", got)
	}
	if got.Empty() {
		t.Fatalf("Empty() = true for an owned block with an explicit (empty) clear: %+v", got)
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
	if _, err := s.SetCandidateOwned(context.Background(), 7, Owned{FullName: "Ada", Links: []string{"https://ada.example"}}); err != nil {
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
