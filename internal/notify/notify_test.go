package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/search"
)

// --- fakes ---------------------------------------------------------------

type fakeSearcher struct {
	// byQuery maps a query's "q" value to the hits it should return; queries are
	// keyed by the parsed q so a test can return different hits per query.
	results []search.SearchResult
	calls   []search.SearchParams
}

func (f *fakeSearcher) Search(_ context.Context, p search.SearchParams) (search.SearchResult, error) {
	f.calls = append(f.calls, p)
	i := len(f.calls) - 1
	if i < len(f.results) {
		return f.results[i], nil
	}
	return search.SearchResult{}, nil
}

type recordedMatch struct{ sub, job int64 }

type fakeStore struct {
	active     []db.ListActiveSubscriptionsRow
	recorded   []recordedMatch
	claimed    []db.ClaimSubscriptionMatchesRow
	delivery   map[int64]db.GetSubscriptionForDeliveryRow
	digestJobs map[int64]db.GetJobsForDigestRow

	notified []db.MarkMatchesNotifiedParams
	failures []db.RecordMatchDeliveryFailureParams
	released []db.ReleaseMatchClaimParams
}

func (s *fakeStore) ListActiveSubscriptions(context.Context) ([]db.ListActiveSubscriptionsRow, error) {
	return s.active, nil
}

func (s *fakeStore) RecordSubscriptionMatch(_ context.Context, a db.RecordSubscriptionMatchParams) (int64, error) {
	for _, m := range s.recorded {
		if m.sub == a.SubscriptionID && m.job == a.JobID {
			return 0, nil // already recorded → idempotent no-op
		}
	}
	s.recorded = append(s.recorded, recordedMatch{a.SubscriptionID, a.JobID})
	return 1, nil
}

func (s *fakeStore) ClaimSubscriptionMatches(context.Context, db.ClaimSubscriptionMatchesParams) ([]db.ClaimSubscriptionMatchesRow, error) {
	return s.claimed, nil
}

func (s *fakeStore) GetSubscriptionForDelivery(_ context.Context, id int64) (db.GetSubscriptionForDeliveryRow, error) {
	return s.delivery[id], nil
}

func (s *fakeStore) GetJobsForDigest(_ context.Context, ids []int64) ([]db.GetJobsForDigestRow, error) {
	out := make([]db.GetJobsForDigestRow, 0, len(ids))
	for _, id := range ids {
		if j, ok := s.digestJobs[id]; ok {
			out = append(out, j)
		}
	}
	return out, nil
}

func (s *fakeStore) MarkMatchesNotified(_ context.Context, a db.MarkMatchesNotifiedParams) (int64, error) {
	s.notified = append(s.notified, a)
	return int64(len(a.JobIds)), nil
}

func (s *fakeStore) RecordMatchDeliveryFailure(_ context.Context, a db.RecordMatchDeliveryFailureParams) error {
	s.failures = append(s.failures, a)
	return nil
}

func (s *fakeStore) ReleaseMatchClaim(_ context.Context, a db.ReleaseMatchClaimParams) error {
	s.released = append(s.released, a)
	return nil
}

type fakeNotifier struct {
	err  error
	sent []Digest
}

func (n *fakeNotifier) Send(_ context.Context, _, _ string, d Digest) error {
	if n.err != nil {
		return n.err
	}
	n.sent = append(n.sent, d)
	return nil
}

// --- helpers -------------------------------------------------------------

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func rfc(t time.Time) *string { s := t.Format(time.RFC3339); return &s }

func hit(id int64, created time.Time) search.JobDocument {
	return search.JobDocument{ID: id, Job: jobview.Job{CreatedAt: rfc(created)}}
}

// --- tests ---------------------------------------------------------------

func TestMatch_SharedQueryHitsIndexOnce(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, Query: "seniority=senior", StartAt: ts(base)},
			{ID: 2, Query: "seniority=senior", StartAt: ts(base)},
		},
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hit(100, base.Add(time.Hour))}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(searcher.calls) != 1 {
		t.Errorf("search calls = %d, want 1 (the shared query is queried once)", len(searcher.calls))
	}
	// Both subscriptions on the shared query get the match.
	if len(store.recorded) != 2 {
		t.Fatalf("recorded matches = %d, want 2", len(store.recorded))
	}
}

func TestMatch_StartAtGate(t *testing.T) {
	cutoff := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, Query: "q=go", StartAt: ts(cutoff)},
		},
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{
			hit(10, cutoff.Add(-time.Hour)), // before the cutoff → not recorded
			hit(11, cutoff.Add(time.Hour)),  // after the cutoff → recorded
		}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 1 || store.recorded[0].job != 11 {
		t.Errorf("recorded = %+v, want only job 11 (after start_at)", store.recorded)
	}
}

func TestDeliver_OneDigestPerSubscription(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{
			{SubscriptionID: 1, JobID: 10},
			{SubscriptionID: 1, JobID: 11},
			{SubscriptionID: 2, JobID: 12},
		},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, Channel: ChannelTelegram, SavedSearchName: "Go", TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
			2: {ID: 2, Channel: ChannelTelegram, SavedSearchName: "Rust", TelegramChatID: pgtype.Int8{Int64: 666, Valid: true}},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{
			10: {ID: 10, Title: "A"}, 11: {ID: 11, Title: "B"}, 12: {ID: 12, Title: "C"},
		},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 2 {
		t.Fatalf("digests sent = %d, want 2 (one per subscription)", len(notifier.sent))
	}
	// Subscription 1's digest carries both its jobs; all claimed marked notified.
	if notifier.sent[0].Total != 2 || len(notifier.sent[0].Jobs) != 2 {
		t.Errorf("first digest = %+v, want 2 jobs", notifier.sent[0])
	}
	if len(store.notified) != 2 {
		t.Errorf("mark-notified calls = %d, want 2", len(store.notified))
	}
	if len(store.failures) != 0 {
		t.Errorf("failures = %d, want 0", len(store.failures))
	}
}

func TestDeliver_FailureIsRecordedNotNotified(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true}},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{err: errors.New("telegram down")}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.failures) != 1 {
		t.Errorf("failures = %d, want 1", len(store.failures))
	}
	if len(store.notified) != 0 {
		t.Errorf("notified = %d, want 0 (a failed delivery must stay pending)", len(store.notified))
	}
}

func TestDeliver_UnlinkedTelegramIsSoftSkipped(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Valid: false}}, // not linked
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("sent = %d, want 0 (unlinked → no send)", len(notifier.sent))
	}
	if len(store.released) != 1 {
		t.Errorf("released = %d, want 1 (claim released for retry)", len(store.released))
	}
	if len(store.failures) != 0 {
		t.Errorf("failures = %d, want 0 (soft-skip is not a failed attempt)", len(store.failures))
	}
	if stats.SoftSkips != 1 {
		t.Errorf("soft skips = %d, want 1", stats.SoftSkips)
	}
}
