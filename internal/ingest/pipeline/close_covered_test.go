package pipeline

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// closingStore is a fakeStore that also implements CoveredCloser.
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

// A covered posting's stored row is closed, not refreshed, while an unreadable one keeps the
// liveness refresh; both count as accounted for so the provider still sweeps.
func TestRunClosesTheStoredRowsTheCoverageGateSkips(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
		{ExternalID: "2", Title: "Data Engineer", Company: "Uncovered", Unreadable: true},
		{ExternalID: "3", Title: "Frontend Engineer", Company: "Uncovered"},
	}}
	store := &closingStore{fakeStore: &fakeStore{}}
	r := Runner{Registry: registry(src), Store: store, Coverage: &fakeCoverage{covered: map[string]bool{"acme": true}}}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Himalayas", Provider: "himalayas"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := store.closedCover["himalayas"]; !slices.Equal(got, []string{":1"}) {
		t.Errorf("closed covered = %v, want [:1]", got)
	}
	if got := store.refreshed["himalayas"]; !slices.Equal(got, []string{":2"}) {
		t.Errorf("refreshed = %v, want [:2] — only the unreadable posting", got)
	}
	if got := stats.Total().Refreshed; got != 2 {
		t.Errorf("Refreshed = %d, want 2", got)
	}
	if len(store.saved) != 1 {
		t.Errorf("saved = %d, want 1 (the uncovered readable posting)", len(store.saved))
	}
}

// The streaming path shares saveOne, so it closes covered rows the same way.
func TestRunClosesStreamedRowsTheCoverageGateSkips(t *testing.T) {
	src := fakeStreamingSource{provider: "jobtech", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}}
	store := &closingStore{fakeStore: &fakeStore{}}
	r := Runner{Registry: registry(src), Store: store, Coverage: &fakeCoverage{covered: map[string]bool{"acme": true}}}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "jobtech", Board: "acme"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := store.closedCover["jobtech"]; !slices.Equal(got, []string{"acme:1"}) {
		t.Errorf("closed covered = %v, want [acme:1]", got)
	}
	if len(store.refreshed) != 0 {
		t.Errorf("refreshed = %v, want none", store.refreshed)
	}
}

// A hydrating source's re-listed posting of a covered employer is closed instead of touched;
// an uncovered one is still touched.
func TestRunClosesSeenRefreshRowsTheCoverageGateCovers(t *testing.T) {
	src := fakeSource{provider: "himalayas", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme", SeenRefresh: true},
		{ExternalID: "2", Title: "Backend Engineer", Company: "Uncovered", SeenRefresh: true},
	}}
	store := &closingStore{fakeStore: &fakeStore{}}
	r := Runner{Registry: registry(src), Store: store, Coverage: &fakeCoverage{covered: map[string]bool{"acme": true}}}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Himalayas", Provider: "himalayas"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := store.closedCover["himalayas"]; !slices.Equal(got, []string{":1"}) {
		t.Errorf("closed covered = %v, want [:1]", got)
	}
	if len(store.touched) != 1 || store.touched[0][1] != ":2" {
		t.Errorf("touched = %v, want only :2", store.touched)
	}
}
