package notify

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

func TestMatch_WatchlistQueryIsSearchedPerSubscriberOverTheirOwnCompanies(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, UserID: 1, Query: "watchlist=true", StartAt: ts(base)},
			{ID: 2, UserID: 2, Query: "watchlist=true", StartAt: ts(base)},
		},
		watchlists: map[int64][]string{1: {"acme"}, 2: {"globex", "initech"}},
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hit(100, base.Add(time.Hour))}},
		{Hits: []search.JobDocument{hit(101, base.Add(time.Hour))}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(searcher.calls) != 2 {
		t.Fatalf("searches = %d, want one per subscriber", len(searcher.calls))
	}
	got := map[string]bool{}
	for _, c := range searcher.calls {
		got[reflect.ValueOf(c.Filter).Index(0).Index(0).String()] = true
	}
	for _, want := range []string{`company_slug IN ["acme"]`, `company_slug IN ["globex", "initech"]`} {
		if !got[want] {
			t.Errorf("no search filtered by %s; filters = %v", want, got)
		}
	}
	// Each subscriber is recorded only the job their own search found.
	if len(store.recorded) != 2 || stats.FailedQueries != 0 {
		t.Errorf("recorded = %+v, failed = %d", store.recorded, stats.FailedQueries)
	}
}

func TestMatch_WatchlistFilterCombinesWithTheQueryAndKeepsOtherSubscribersShared(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, UserID: 1, Query: "seniority=senior&watchlist=true", StartAt: ts(base)},
			{ID: 2, UserID: 2, Query: "seniority=senior", StartAt: ts(base)},
			{ID: 3, UserID: 3, Query: "seniority=senior", StartAt: ts(base)},
		},
		watchlists: map[int64][]string{1: {"acme"}},
	}
	searcher := &fakeSearcher{}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	// One shared search for the two plain subscribers plus one for the watcher.
	if len(searcher.calls) != 2 {
		t.Fatalf("searches = %d, want 2", len(searcher.calls))
	}
	var narrowed bool
	for _, c := range searcher.calls {
		groups, _ := c.Filter.([][]string)
		if reflect.DeepEqual(groups, [][]string{{`enrichment.seniority = "senior"`}, {`company_slug IN ["acme"]`}}) {
			narrowed = true
		}
	}
	if !narrowed {
		t.Errorf("no search carried both the seniority filter and the watchlist: %+v", searcher.calls)
	}
}

func TestMatch_EmptyWatchlistSearchesNothingRatherThanTheWholeCatalogue(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, UserID: 1, Query: "watchlist=true", StartAt: ts(base)},
		},
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hit(100, base.Add(time.Hour))}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(searcher.calls) != 0 || len(store.recorded) != 0 {
		t.Errorf("searches = %d, recorded = %+v; an empty watchlist must match nothing", len(searcher.calls), store.recorded)
	}
	if stats.FailedQueries != 0 {
		t.Errorf("failed = %d, want 0 — an empty list is not a failure", stats.FailedQueries)
	}
	if len(store.searched) != 1 {
		t.Errorf("subscription not stamped searched: %+v", store.searched)
	}
}
