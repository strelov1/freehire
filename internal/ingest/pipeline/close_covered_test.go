package pipeline

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// closingStore is a fakeStore that exposes the historical company-wide closer as a regression sentinel.
type closingStore struct {
	*fakeStore
	cmu         sync.Mutex
	closedCover map[string][]string // source -> external ids passed to CloseCovered
}

func (s *closingStore) CloseCovered(_ context.Context, source string, externalIDs []string) ([]string, error) {
	s.cmu.Lock()
	defer s.cmu.Unlock()
	if s.closedCover == nil {
		s.closedCover = map[string][]string{}
	}
	s.closedCover[source] = append(s.closedCover[source], externalIDs...)
	return make([]string, len(externalIDs)), nil
}

// Readable postings are saved; only unreadable ones get an open-only refresh.
func TestRunKeepsReadablePostingsAlongsideUnreadableOnes(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
		{ExternalID: "2", Title: "Data Engineer", Company: "Uncovered", Unreadable: true},
		{ExternalID: "3", Title: "Frontend Engineer", Company: "Uncovered"},
	}}
	store := &closingStore{fakeStore: &fakeStore{}}
	r := Runner{Registry: registry(src), Store: store}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Himalayas", Provider: "himalayas"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := store.closedCover["himalayas"]; len(got) != 0 {
		t.Errorf("closed covered = %v, want none", got)
	}
	if got := store.refreshed["himalayas"]; !slices.Equal(got, []string{":2"}) {
		t.Errorf("refreshed = %v, want [:2] — only the unreadable posting", got)
	}
	if got := stats.Total().Refreshed; got != 1 {
		t.Errorf("Refreshed = %d, want 1", got)
	}
	if len(store.saved) != 2 {
		t.Errorf("saved = %d, want 2 readable postings", len(store.saved))
	}
}

// Streaming ingestion must preserve postings through the same write path.
func TestRunKeepsStreamedAggregatorPostings(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &closingStore{fakeStore: &fakeStore{}}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "jobtech", Board: "acme"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := store.closedCover["jobtech"]; len(got) != 0 {
		t.Errorf("closed covered = %v, want none", got)
	}
	if len(store.refreshed) != 0 {
		t.Errorf("refreshed = %v, want none", store.refreshed)
	}
}

// Every re-listed identity is touched rather than closed because of its employer.
func TestRunTouchesEveryRelistedAggregatorPosting(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme", SeenRefresh: true},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Uncovered", SeenRefresh: true},
	}}
	store := &closingStore{fakeStore: &fakeStore{}}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Himalayas", Provider: "himalayas"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := store.closedCover["himalayas"]; len(got) != 0 {
		t.Errorf("closed covered = %v, want none", got)
	}
	if len(store.touched) != 2 {
		t.Errorf("touched = %v, want both postings", store.touched)
	}
}
