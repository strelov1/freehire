package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// recentSource answers Fetch and FetchRecent with different postings so a test can tell which
// one the runner called, and records the window it was handed.
type recentSource struct {
	fakeSource
	recentJobs   []sources.Job
	recentWindow *time.Duration
	fetchCalls   *int
}

func (s recentSource) Fetch(ctx context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	*s.fetchCalls++
	return s.fakeSource.Fetch(ctx, e)
}

func (s recentSource) FetchRecent(_ context.Context, _ sources.CompanyEntry, w time.Duration) ([]sources.Job, error) {
	*s.recentWindow = w
	return s.recentJobs, nil
}

var recentEntry = sources.CompanyEntry{Company: "Acme", Provider: "greenhouse", Board: "acme"}

func newRecentSource() (recentSource, *time.Duration, *int) {
	var w time.Duration
	var calls int
	return recentSource{
		fakeSource:   fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "full", Title: "Go Developer", Company: "Acme"}}},
		recentJobs:   []sources.Job{{ExternalID: "new", Title: "Go Developer", Company: "Acme"}},
		recentWindow: &w,
		fetchCalls:   &calls,
	}, &w, &calls
}

func TestRecentRunCallsFetchRecentWithTheWindow(t *testing.T) {
	src, window, fetchCalls := newRecentSource()
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store, RecentWindow: 72 * time.Hour}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{recentEntry}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if *window != 72*time.Hour {
		t.Errorf("FetchRecent window = %v, want 72h", *window)
	}
	if *fetchCalls != 0 {
		t.Errorf("Fetch called %d times in a recent run, want 0", *fetchCalls)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().ExternalID != "acme:new" {
		t.Errorf("saved = %+v, want only the recent posting", store.saved)
	}
}

func TestFullRunIgnoresFetchRecent(t *testing.T) {
	src, window, fetchCalls := newRecentSource()
	store := &fakeStore{}
	r := Runner{Registry: registry(src), Store: store}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{recentEntry}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if *window != 0 || *fetchCalls != 1 {
		t.Errorf("window = %v, Fetch calls = %d; want a plain Fetch only", *window, *fetchCalls)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().ExternalID != "acme:full" {
		t.Errorf("saved = %+v, want the full-listing posting", store.saved)
	}
}

func TestRecentRunSkipsAndCountsAnAdapterWithoutADateFilter(t *testing.T) {
	src := fakeSource{provider: "greenhouse", jobs: []sources.Job{{ExternalID: "1", Title: "Go Developer", Company: "Acme"}}}
	store := &fakeStore{}
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: store, BoardHealth: health, RecentWindow: 24 * time.Hour}

	stats, err := r.Run(context.Background(), []sources.CompanyEntry{recentEntry})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := stats["greenhouse"]
	if got.RecentUnsupported != 1 || got.Ingested != 0 || got.Failed != 0 {
		t.Errorf("stats = %+v, want RecentUnsupported=1 and nothing ingested or failed", got)
	}
	if len(store.saved) != 0 {
		t.Errorf("saved = %+v, want nothing: no hidden full crawl", store.saved)
	}
	if len(health.successes)+len(health.failures) != 0 {
		t.Errorf("health recorded %v / %v, want nothing", health.successes, health.failures)
	}
}

func TestRecentRunRecordsNoBoardHealth(t *testing.T) {
	src, _, _ := newRecentSource()
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health, RecentWindow: 24 * time.Hour}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{recentEntry}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(health.successes) != 0 || len(health.failures) != 0 || len(health.reached) != 0 {
		t.Errorf("recent run wrote health: successes=%v failures=%v reached=%v", health.successes, health.failures, health.reached)
	}
}

func TestRecentRunDoesNotRecoverACooledProvider(t *testing.T) {
	src, _, _ := newRecentSource()
	health := &fakeHealth{cooldowns: map[string]time.Time{healthKey("greenhouse", "a", ""): time.Now().Add(time.Hour)}}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health, RecentWindow: 24 * time.Hour}

	entries := []sources.CompanyEntry{
		{Company: "A", Provider: "greenhouse", Board: "a"},
		{Company: "B", Provider: "greenhouse", Board: "b"},
	}
	if _, err := r.Run(context.Background(), entries); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(health.cleared) != 0 {
		t.Errorf("cleared cooldowns for %v, want none", health.cleared)
	}
}

func TestFullRunStillRecordsBoardHealth(t *testing.T) {
	src, _, _ := newRecentSource()
	health := &fakeHealth{}
	r := Runner{Registry: registry(src), Store: &fakeStore{}, BoardHealth: health}

	if _, err := r.Run(context.Background(), []sources.CompanyEntry{recentEntry}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(health.successes) != 1 {
		t.Errorf("successes = %v, want one", health.successes)
	}
}
