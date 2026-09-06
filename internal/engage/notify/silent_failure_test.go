package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// A pass in which every saved-search query failed printed `queries=53 matched=0 ...
// failed=0` and exited 0 — byte-identical to a pass with nothing new to match. The
// isolation is deliberate (one bad filter must not fail the other fifty-two), but the
// count is what makes it reportable, and there was none.
func TestMatch_CountsEveryFailedQuerySoACollapsedPassIsVisible(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{active: []db.ListActiveSubscriptionsRow{
		{ID: 1, Query: "seniority=senior", StartAt: ts(base)},
		{ID: 2, Query: "seniority=junior", StartAt: ts(base)},
	}}
	searcher := &fakeSearcher{err: errors.New("meilisearch: connection refused")}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	stats, err := r.Run(context.Background())

	// Still not fatal: the pass goes on to DELIVER whatever was already pending.
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Queries != 2 || stats.FailedQueries != 2 {
		t.Errorf("stats = %d queries / %d failed, want 2/2", stats.Queries, stats.FailedQueries)
	}
	if !stats.MatchingCollapsed() {
		t.Error("a pass whose every query failed must report matching as collapsed, or the worker exits 0")
	}
}

// The other half of the same rule: one broken saved search must not turn the run red for
// the fifty-two it served. Only ALL of them failing says the index or its credential is
// gone.
func TestMatch_OneFailedQueryDoesNotCollapseThePass(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{active: []db.ListActiveSubscriptionsRow{
		{ID: 1, Query: "seniority=senior", StartAt: ts(base)},
		{ID: 2, Query: "seniority=junior", StartAt: ts(base)},
	}}
	searcher := &fakeSearcher{
		errs:    []error{errors.New("invalid filter")},
		results: []search.SearchResult{{}, {Hits: []search.JobDocument{hit(100, base.Add(time.Hour))}}},
	}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.FailedQueries != 1 {
		t.Errorf("failed queries = %d, want 1", stats.FailedQueries)
	}
	if stats.MatchingCollapsed() {
		t.Error("one failed query out of two must not collapse the pass")
	}
	if stats.Matched != 1 {
		t.Errorf("matched = %d, want 1 — the surviving query still recorded its hit", stats.Matched)
	}
}

// A cancelled pass must stop rather than walk the remaining queries against a dead
// context: each would fail instantly and be counted, so one SIGTERM would report as
// "every saved search is broken" — which is also exactly what MatchingCollapsed would
// then turn into a red exit code.
func TestMatch_CancellationStopsInsteadOfCountingEveryQueryFailed(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{active: []db.ListActiveSubscriptionsRow{
		{ID: 1, Query: "seniority=senior", StartAt: ts(base)},
		{ID: 2, Query: "seniority=junior", StartAt: ts(base)},
		{ID: 3, Query: "seniority=lead", StartAt: ts(base)},
	}}
	searcher := &fakeSearcher{}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats, err := r.Run(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want it to carry context.Canceled", err)
	}
	if stats.FailedQueries != 0 {
		t.Errorf("failed queries = %d, want 0 — a cancellation is one event, not %d broken searches",
			stats.FailedQueries, stats.FailedQueries)
	}
	if len(searcher.calls) != 1 {
		t.Errorf("search calls = %d, want 1: the loop must break, not run every query into the dead context",
			len(searcher.calls))
	}
}

// deliverOne's two reads happen before anything is sent and release the claim on
// failure, so they burn no attempt and leave no dead letter. Without a counter a pass
// that could not read a single subscription printed `delivered=0 failed=0` and exited 0,
// which reads as an idle queue.
func TestDeliver_CountsASubscriptionItCouldNotRead(t *testing.T) {
	store := &fakeStore{
		claimed:     []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		deliveryErr: errors.New("connection reset"),
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Failed != 1 {
		t.Errorf("failed = %d, want 1", stats.Failed)
	}
	if len(store.released) != 1 {
		t.Errorf("released claims = %d, want 1 — the matches must stay pending for the next pass", len(store.released))
	}
}

func TestDeliver_CountsADigestWhoseJobsCouldNotBeRead(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
		},
		digestJobsErr: errors.New("statement timeout"),
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Failed != 1 {
		t.Errorf("failed = %d, want 1", stats.Failed)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("digests sent = %d, want 0", len(notifier.sent))
	}
}

// The delivery half of the cancellation rule: a stopped run must not report an outage
// across every claimed subscription. Their leases expire and the next pass retries them.
func TestDeliver_CancellationStopsWalkingClaimedSubscriptions(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{
			{SubscriptionID: 1, JobID: 10},
			{SubscriptionID: 2, JobID: 11},
			{SubscriptionID: 3, JobID: 12},
		},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
			2: {ID: 2, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 666, Valid: true}},
			3: {ID: 3, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 777, Valid: true}},
		},
		digestJobsErr: errors.New("connection reset"),
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats, err := r.Run(ctx)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Failed != 0 {
		t.Errorf("failed = %d, want 0 — a cancelled run has not observed any delivery failing", stats.Failed)
	}
}
