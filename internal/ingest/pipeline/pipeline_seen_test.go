package pipeline

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// TestRunDrivesHydratingSourceWithSeenSet proves the runner prefers FetchNew for a
// HydratingSource and supplies a seen predicate backed by the store's set of already-ingested
// (namespaced) external_ids: the boardless offer already stored as ":seen" reads seen=true, a
// new offer reads seen=false.
func TestRunDrivesHydratingSourceWithSeenSet(t *testing.T) {
	src := &fakeHydratingSource{provider: "justjoin", jobs: []sources.Job{
		{ExternalID: "seen", Title: "A", Company: "C", URL: "u"},
		{ExternalID: "new", Title: "B", Company: "C", URL: "u"},
	}}
	store := &fakeStore{seenIDs: map[string]bool{":seen": true}}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "justjoin", Board: ""},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !src.fetchNewCalled {
		t.Fatal("runner should call FetchNew for a HydratingSource, not Fetch")
	}
	if !src.seenResults["seen"] {
		t.Error("seen(\"seen\") = false, want true (already stored as \":seen\")")
	}
	if src.seenResults["new"] {
		t.Error("seen(\"new\") = true, want false (not yet stored)")
	}
	// The new offer is upserted; the seen offer is only touched (liveness refresh), so its
	// hydrated content is never overwritten by a content-less re-upsert. Boardless → ":<id>".
	if len(store.saved) != 1 || store.saved[0].Fields().ExternalID != ":new" {
		t.Errorf("saved = %+v, want only the new offer (\":new\")", store.saved)
	}
	if len(store.touched) != 1 || store.touched[0] != [2]string{"justjoin", ":seen"} {
		t.Errorf("touched = %v, want one touch of (justjoin, :seen)", store.touched)
	}
}

// TestRunScopesSeenSetToTheCrawledBoard proves the seen-set of a multi-board provider is read per
// board, not per provider: each board's lookup is scoped to its own board id, so a posting stored
// under a sibling board is NOT reported as seen. Without the scope a provider-wide read would cost
// the whole catalogue on every one of the provider's boards.
func TestRunScopesSeenSetToTheCrawledBoard(t *testing.T) {
	src := &fakeHydratingSource{provider: "workday", jobs: []sources.Job{
		{ExternalID: "1", Title: "A", Company: "C", URL: "u"},
	}}
	store := &fakeStore{seenByBrd: map[string]map[string]bool{
		"boardA": {"boardA:1": true},
		"boardB": {"boardB:9": true},
	}}
	r := Runner{Registry: registry(src), Store: store}

	// Crawl boardB, whose stored posting is a different id: posting "1" belongs to boardA and
	// must not read as seen here.
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "workday", Board: "boardB"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !slices.Equal(store.seenAsked, []string{"boardB"}) {
		t.Errorf("seen-set scoped to %v, want [boardB]", store.seenAsked)
	}
	if src.seenResults["1"] {
		t.Error("seen(\"1\") = true, want false — boardA:1 belongs to another board")
	}
	if len(store.saved) != 1 || store.saved[0].Fields().ExternalID != "boardB:1" {
		t.Errorf("saved = %+v, want the posting hydrated as boardB:1", store.saved)
	}
}

// TestRunHydratingSourceFailsOpenOnSeenLookupError proves a seen-set query failure does not skip
// the board: the runner falls back to an empty seen-set (every offer treated as new) and still
// crawls via FetchNew.
func TestRunHydratingSourceFailsOpenOnSeenLookupError(t *testing.T) {
	src := &fakeHydratingSource{provider: "justjoin", jobs: []sources.Job{
		{ExternalID: "a", Title: "A", Company: "C", URL: "u"},
	}}
	store := &fakeStore{seenErr: errors.New("db down")}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "C", Provider: "justjoin", Board: ""},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !src.fetchNewCalled {
		t.Fatal("runner should still crawl via FetchNew despite the seen-lookup error")
	}
	if src.seenResults["a"] {
		t.Error("seen(\"a\") = true, want false (empty set on lookup error)")
	}
	if len(store.saved) != 1 {
		t.Errorf("len(saved) = %d, want 1 (board still crawled)", len(store.saved))
	}
}

// A board crawled without its seen-set proves nothing about what is gone: every stored posting
// reads as new, so a budgeted adapter yields only its budget of them and the rest go untouched.
// The run says so (SeenUnknown), the board does not qualify for the board scope, and its
// companies are withheld from the company scope — otherwise the sweep would close live postings
// the crawl listed but never refreshed.
func TestRunSeenLookupErrorWithholdsTheBoardsCloses(t *testing.T) {
	src := &fakeHydratingSource{provider: "justjoin", jobs: []sources.Job{
		{ExternalID: "a", Title: "A", Company: "Acme", URL: "u"},
	}}
	store := &fakeStore{seenErr: errors.New("db down")}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{
		{Company: "Acme", Provider: "justjoin", Board: "b1"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	st := stats["justjoin"]
	if st.SeenUnknown != 1 {
		t.Errorf("SeenUnknown = %d, want 1", st.SeenUnknown)
	}
	if len(st.QualifyingBoards) != 0 {
		t.Errorf("QualifyingBoards = %v, want none: a board crawled without its seen-set proves nothing", st.QualifyingBoards)
	}
	if !slices.Contains(st.UnprovenCompanies, "acme") {
		t.Errorf("UnprovenCompanies = %v, want acme withheld from the company scope", st.UnprovenCompanies)
	}
	if !slices.Contains(st.SkippedBoards, externalid.BoardPattern("b1")) {
		t.Errorf("SkippedBoards = %v, want b1 excluded from the company and source scopes", st.SkippedBoards)
	}
}
