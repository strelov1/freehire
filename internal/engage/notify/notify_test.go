package notify

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
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
	// unlinkedTelegram records the user ids a delivery forgot the Telegram chat
	// for, so a test can assert the 403 path unlinks exactly once per user.
	unlinkedTelegram  []int64
	unlinkTelegramErr error

	active      []db.ListActiveSubscriptionsRow
	recorded    []recordedMatch
	recordCalls int // how many times RecordSubscriptionMatches was called, not how many pairs
	claimed     []db.ClaimSubscriptionMatchesRow
	delivery    map[int64]db.GetSubscriptionForDeliveryRow
	digestJobs  map[int64]db.GetJobsForDigestRow

	// excludedSkills maps a user id to their avoid-skills preference; a user id absent from
	// this map has no profile row, mirroring the real query.
	excludedSkills map[int64][]string

	notified   []db.MarkMatchesNotifiedParams
	failures   []db.RecordMatchDeliveryFailureParams
	released   []db.ReleaseMatchClaimParams
	digestSent []int64

	recordedNotifications []db.RecordNotificationParams
	recordNotificationErr error
	// nextNotificationID is the id the next RecordNotification hands back, so a
	// test can assert the digest carried it; deletedNotifications records the
	// ids a failed send withdrew.
	nextNotificationID   int64
	deletedNotifications []int64
}

func (s *fakeStore) ListActiveSubscriptions(context.Context) ([]db.ListActiveSubscriptionsRow, error) {
	return s.active, nil
}

func (s *fakeStore) ListUserProfilesExcludedSkills(_ context.Context, userIDs []int64) ([]db.ListUserProfilesExcludedSkillsRow, error) {
	out := make([]db.ListUserProfilesExcludedSkillsRow, 0, len(userIDs))
	for _, id := range userIDs {
		skills, ok := s.excludedSkills[id]
		if !ok {
			continue
		}
		out = append(out, db.ListUserProfilesExcludedSkillsRow{UserID: id, ExcludedSkills: skills})
	}
	return out, nil
}

func (s *fakeStore) RecordSubscriptionMatches(_ context.Context, a db.RecordSubscriptionMatchesParams) (int64, error) {
	s.recordCalls++
	var n int64
	for i, subID := range a.SubscriptionIds {
		jobID := a.JobIds[i]
		known := false
		for _, m := range s.recorded {
			if m.sub == subID && m.job == jobID {
				known = true // already recorded → idempotent no-op
				break
			}
		}
		if known {
			continue
		}
		s.recorded = append(s.recorded, recordedMatch{subID, jobID})
		n++
	}
	return n, nil
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

func (s *fakeStore) MarkDigestSent(_ context.Context, id int64) error {
	s.digestSent = append(s.digestSent, id)
	return nil
}

func (s *fakeStore) RecordNotification(_ context.Context, a db.RecordNotificationParams) (int64, error) {
	s.recordedNotifications = append(s.recordedNotifications, a)
	if s.recordNotificationErr != nil {
		return 0, s.recordNotificationErr
	}
	if s.nextNotificationID == 0 {
		s.nextNotificationID = 1
	}
	id := s.nextNotificationID
	s.nextNotificationID++
	return id, nil
}

func (s *fakeStore) DeleteNotification(_ context.Context, id int64) error {
	s.deletedNotifications = append(s.deletedNotifications, id)
	return nil
}

func (s *fakeStore) DeleteTelegramLink(_ context.Context, userID int64) (int64, error) {
	s.unlinkedTelegram = append(s.unlinkedTelegram, userID)
	if s.unlinkTelegramErr != nil {
		return 0, s.unlinkTelegramErr
	}
	return 1, nil
}

type fakeNotifier struct {
	err  error
	sent []Digest
}

// Send records the digest even when it fails, so a test can assert what the
// channel was handed on the failure path too.
func (n *fakeNotifier) Send(_ context.Context, _, _ string, d Digest) error {
	n.sent = append(n.sent, d)
	return n.err
}

// --- helpers -------------------------------------------------------------

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func rfc(t time.Time) *string { s := t.Format(time.RFC3339); return &s }

func hit(id int64, created time.Time) search.JobDocument {
	return search.JobDocument{ID: id, Job: jobview.Job{CreatedAt: rfc(created)}}
}

func hitWithSkills(id int64, created time.Time, skills ...string) search.JobDocument {
	return search.JobDocument{ID: id, Job: jobview.Job{CreatedAt: rfc(created), Skills: skills}}
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

// One query, many subscribers, many hits: the review finding was that matchQuery issued
// one sequential RecordSubscriptionMatch round trip per (hit, subscription) pair. It must
// now record the whole batch in a single call, regardless of how many pairs that is.
func TestMatch_BatchesMatchesIntoOneCallPerQuery(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, Query: "seniority=senior", StartAt: ts(base)},
			{ID: 2, Query: "seniority=senior", StartAt: ts(base)},
			{ID: 3, Query: "seniority=senior", StartAt: ts(base)},
		},
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{
			hit(100, base.Add(time.Hour)),
			hit(101, base.Add(2*time.Hour)),
		}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if store.recordCalls != 1 {
		t.Errorf("RecordSubscriptionMatches calls = %d, want 1 for 3 subscribers x 2 hits", store.recordCalls)
	}
	if len(store.recorded) != 6 {
		t.Fatalf("recorded matches = %d, want 6 (3 subscriptions x 2 hits)", len(store.recorded))
	}
	if stats.Matched != 6 {
		t.Errorf("stats.Matched = %d, want 6", stats.Matched)
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

func TestMatch_ExcludedSkillIsNotRecorded(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, UserID: 1, Query: "seniority=senior", StartAt: ts(base)},
		},
		excludedSkills: map[int64][]string{1: {"php"}},
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hitWithSkills(100, base.Add(time.Hour), "php", "go")}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 0 {
		t.Errorf("recorded = %+v, want none (job carries the subscriber's avoided skill)", store.recorded)
	}
}

func TestMatch_ExcludedSkillsArePerSubscriberNotPerQuery(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, UserID: 1, Query: "seniority=senior", StartAt: ts(base)},
			{ID: 2, UserID: 2, Query: "seniority=senior", StartAt: ts(base)},
		},
		excludedSkills: map[int64][]string{1: {"php"}}, // user 2 has no avoid list
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hitWithSkills(100, base.Add(time.Hour), "php")}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 1 || store.recorded[0].sub != 2 {
		t.Errorf("recorded = %+v, want only subscription 2 (subscriber 1 avoids php)", store.recorded)
	}
}

func TestMatch_ExcludedSkillsReflectLatestProfileWithoutRecreatingSubscription(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, UserID: 1, Query: "seniority=senior", StartAt: ts(base)},
		},
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hitWithSkills(100, base.Add(time.Hour), "php")}},
		{Hits: []search.JobDocument{hitWithSkills(101, base.Add(2*time.Hour), "php")}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	// First pass: no avoid list yet, the job matches.
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 1 {
		t.Fatalf("after first pass recorded = %+v, want 1 match", store.recorded)
	}

	// The subscriber adds "php" to their avoid list without touching the subscription.
	store.excludedSkills = map[int64][]string{1: {"php"}}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 1 {
		t.Errorf("after avoid-list update recorded = %+v, want still 1 (no new php match)", store.recorded)
	}
}

func TestMatch_SubscriberWithNoProfileMatchesNormally(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, UserID: 1, Query: "seniority=senior", StartAt: ts(base)},
		},
		// No entry for user 1 in excludedSkills — mirrors a user with no saved profile.
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hitWithSkills(100, base.Add(time.Hour), "php")}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 1 {
		t.Errorf("recorded = %+v, want 1 (no profile means no exclusion)", store.recorded)
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

func TestDeliver_EmailResolvesAccountEmail(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, Channel: ChannelEmail, SavedSearchName: "Go", AccountEmail: "user@acme.com"},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A"}},
	}
	em := &recordingNotifier{}
	r := New(store, &fakeSearcher{}, Router{ChannelEmail: em}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(em.sent) != 1 || em.dest != "user@acme.com" {
		t.Errorf("email notifier got %d sends dest=%q, want 1 to the account email", len(em.sent), em.dest)
	}
	if len(store.notified) != 1 {
		t.Errorf("notified = %d, want 1", len(store.notified))
	}
}

func TestDeliver_UnconfiguredEmailChannelIsSoftSkipped(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, Channel: ChannelEmail, AccountEmail: "user@acme.com"},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	// Router without an email notifier (SES unconfigured): the email channel is
	// unregistered, so delivery must soft-skip rather than dead-letter.
	r := New(store, &fakeSearcher{}, Router{ChannelTelegram: &recordingNotifier{}}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.failures) != 0 {
		t.Errorf("failures = %d, want 0 (unconfigured channel is a soft-skip, not a failed attempt)", len(store.failures))
	}
	if len(store.released) != 1 {
		t.Errorf("released = %d, want 1 (claim released for retry)", len(store.released))
	}
	if stats.SoftSkips != 1 {
		t.Errorf("soft skips = %d, want 1", stats.SoftSkips)
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

func TestDeliver_RecordsNotificationForSingleJobDigest(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, UserID: 42, Channel: ChannelTelegram, SavedSearchName: "Go", TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "acme-go-engineer"}},
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recordedNotifications) != 1 {
		t.Fatalf("recorded notifications = %d, want 1", len(store.recordedNotifications))
	}
	rec := store.recordedNotifications[0]
	if rec.UserID != 42 {
		t.Errorf("UserID = %d, want 42", rec.UserID)
	}
	if rec.Kind != "subscription_digest" {
		t.Errorf("Kind = %q, want %q", rec.Kind, "subscription_digest")
	}
	if !rec.PublicSlug.Valid || rec.PublicSlug.String != "acme-go-engineer" {
		t.Errorf("PublicSlug = %+v, want valid %q", rec.PublicSlug, "acme-go-engineer")
	}
}

func TestDeliver_RecordsNotificationForMultiJobDigestWithoutSlug(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{
			{SubscriptionID: 1, JobID: 10},
			{SubscriptionID: 1, JobID: 11},
		},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, UserID: 42, Channel: ChannelTelegram, SavedSearchName: "Go", TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{
			10: {ID: 10, Title: "A", PublicSlug: "a"},
			11: {ID: 11, Title: "B", PublicSlug: "b"},
		},
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recordedNotifications) != 1 {
		t.Fatalf("recorded notifications = %d, want 1", len(store.recordedNotifications))
	}
	if store.recordedNotifications[0].PublicSlug.Valid {
		t.Errorf("PublicSlug = %+v, want invalid (no slug) for a multi-job digest", store.recordedNotifications[0].PublicSlug)
	}

	var jobs []struct {
		Title   string `json:"title"`
		Company string `json:"company"`
		Slug    string `json:"slug"`
	}
	if err := json.Unmarshal(store.recordedNotifications[0].Jobs, &jobs); err != nil {
		t.Fatalf("Jobs did not unmarshal: %v (raw: %s)", err, store.recordedNotifications[0].Jobs)
	}
	want := []struct {
		Title   string `json:"title"`
		Company string `json:"company"`
		Slug    string `json:"slug"`
	}{
		{Title: "A", Company: "", Slug: "a"},
		{Title: "B", Company: "", Slug: "b"},
	}
	if !reflect.DeepEqual(jobs, want) {
		t.Errorf("Jobs = %+v, want %+v", jobs, want)
	}
}

func TestDeliver_RecordNotificationFailureDoesNotBlockDelivery(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, UserID: 42, Channel: ChannelTelegram, SavedSearchName: "Go", TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
		},
		digestJobs:            map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		recordNotificationErr: errors.New("insert failed"),
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Delivered != 1 {
		t.Errorf("Delivered = %d, want 1 (a recording failure must not fail the delivery)", stats.Delivered)
	}
	if len(store.notified) != 1 {
		t.Errorf("notified = %d, want 1 (MarkMatchesNotified must still be called)", len(store.notified))
	}
	if len(store.recordedNotifications) != 1 {
		t.Errorf("recorded notifications = %d, want 1 (the attempt itself still happened)", len(store.recordedNotifications))
	}
}

// ValidChannel is the membership test both create-time gates use. It exists because the slice
// alone is not usable as an allowlist, so subscriptions and reminders each built the same map
// from it — and a third caller would have built a third.
func TestValidChannel(t *testing.T) {
	for _, c := range Channels {
		if !ValidChannel(c) {
			t.Errorf("ValidChannel(%q) = false for a declared channel", c)
		}
	}
	for _, c := range []string{"", "webhook", "Telegram", "e-mail"} {
		if ValidChannel(c) {
			t.Errorf("ValidChannel(%q) = true for an undeclared channel", c)
		}
	}
}

// Push is the third delivery channel, added alongside Telegram and email.
func TestValidChannel_Push(t *testing.T) {
	if ChannelPush != "push" {
		t.Errorf("ChannelPush = %q, want %q", ChannelPush, "push")
	}
	if !ValidChannel(ChannelPush) {
		t.Error("ValidChannel(ChannelPush) = false, want true")
	}
}

// recipient's push case mirrors Telegram's: a live "has device" boolean gates
// whether the subscription is deliverable, and the destination is the user id
// (PushNotifier expands that into N device sends).
func TestRecipient_PushWithDevice(t *testing.T) {
	info := db.GetSubscriptionForDeliveryRow{
		UserID:        42,
		Channel:       ChannelPush,
		HasPushDevice: true,
	}
	dest, ok := recipient(info)
	if !ok {
		t.Fatal("recipient: ok = false, want true when HasPushDevice")
	}
	if dest != "42" {
		t.Errorf("recipient dest = %q, want %q (the user id)", dest, "42")
	}
}

func TestRecipient_PushWithoutDeviceIsNotDeliverable(t *testing.T) {
	info := db.GetSubscriptionForDeliveryRow{
		UserID:        42,
		Channel:       ChannelPush,
		HasPushDevice: false,
	}
	dest, ok := recipient(info)
	if ok {
		t.Errorf("recipient: ok = true, want false when no device is registered")
	}
	if dest != "" {
		t.Errorf("recipient dest = %q, want empty", dest)
	}
}

// --- delivery timing (frequency + quiet hours) ---------------------------

func pgTime(hh, mm int) pgtype.Time {
	return pgtype.Time{Microseconds: int64(hh)*int64(time.Hour/time.Microsecond) + int64(mm)*int64(time.Minute/time.Microsecond), Valid: true}
}

func TestDeliverOne_InstantDeferredDuringQuietHours(t *testing.T) {
	fixedNow := time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC) // inside 22:00-08:00
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true},
				DigestFrequency: "instant", Timezone: pgtype.Text{String: "UTC", Valid: true},
				QuietHoursStart: pgTime(22, 0), QuietHoursEnd: pgTime(8, 0),
			},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
	r.now = func() time.Time { return fixedNow }

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("sent = %d, want 0 (deferred during quiet hours)", len(notifier.sent))
	}
	if len(store.released) != 1 {
		t.Errorf("released = %d, want 1 (claim released to retry later)", len(store.released))
	}
	if len(store.notified) != 0 || len(store.failures) != 0 {
		t.Errorf("notified=%d failures=%d, want 0/0 (a deferral is neither)", len(store.notified), len(store.failures))
	}
}

func TestDeliverOne_InstantDeliversOutsideQuietHours(t *testing.T) {
	fixedNow := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) // outside 22:00-08:00
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true},
				DigestFrequency: "instant", Timezone: pgtype.Text{String: "UTC", Valid: true},
				QuietHoursStart: pgTime(22, 0), QuietHoursEnd: pgTime(8, 0),
			},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
	r.now = func() time.Time { return fixedNow }

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Errorf("sent = %d, want 1 (outside quiet hours)", len(notifier.sent))
	}
}

func TestDeliverOne_DailyDeferredBeforeDigestTime(t *testing.T) {
	fixedNow := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC) // before 09:00
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true},
				DigestFrequency: "daily", DigestTime: pgTime(9, 0), Timezone: pgtype.Text{String: "UTC", Valid: true},
			},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
	r.now = func() time.Time { return fixedNow }

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("sent = %d, want 0 (before the daily digest time)", len(notifier.sent))
	}
	if len(store.released) != 1 {
		t.Errorf("released = %d, want 1", len(store.released))
	}
}

func TestDeliverOne_DailyDueDeliversAndStampsLastSent(t *testing.T) {
	fixedNow := time.Date(2026, 8, 14, 9, 5, 0, 0, time.UTC) // after 09:00
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true},
				DigestFrequency: "daily", DigestTime: pgTime(9, 0), Timezone: pgtype.Text{String: "UTC", Valid: true},
			},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
	r.now = func() time.Time { return fixedNow }

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("sent = %d, want 1 (due, first send today)", len(notifier.sent))
	}
	if len(store.digestSent) != 1 || store.digestSent[0] != 1 {
		t.Errorf("digestSent = %v, want [1] (last_digest_sent_at stamped)", store.digestSent)
	}
}

func TestDeliverOne_DailyAlreadySentTodayDefers(t *testing.T) {
	fixedNow := time.Date(2026, 8, 14, 18, 0, 0, 0, time.UTC)
	sentEarlierToday := time.Date(2026, 8, 14, 9, 5, 0, 0, time.UTC)
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true},
				DigestFrequency: "daily", DigestTime: pgTime(9, 0), Timezone: pgtype.Text{String: "UTC", Valid: true},
				LastDigestSentAt: pgtype.Timestamptz{Time: sentEarlierToday, Valid: true},
			},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
	r.now = func() time.Time { return fixedNow }

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("sent = %d, want 0 (already sent earlier today)", len(notifier.sent))
	}
}

func TestDeliverOne_DailyIgnoresQuietHours(t *testing.T) {
	fixedNow := time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC) // inside quiet hours AND past digest time
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true},
				DigestFrequency: "daily", DigestTime: pgTime(9, 0), Timezone: pgtype.Text{String: "UTC", Valid: true},
				QuietHoursStart: pgTime(22, 0), QuietHoursEnd: pgTime(8, 0),
			},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
	r.now = func() time.Time { return fixedNow }

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Errorf("sent = %d, want 1 (daily digest is exempt from quiet hours)", len(notifier.sent))
	}
}

// --- record-then-send ordering -------------------------------------------

// deliverySubscription is the minimal Telegram-deliverable subscription the
// ordering tests below all need.
func deliverySubscription() map[int64]db.GetSubscriptionForDeliveryRow {
	return map[int64]db.GetSubscriptionForDeliveryRow{
		1: {ID: 1, UserID: 42, Channel: ChannelTelegram, SavedSearchName: "Go", TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
	}
}

func TestDeliver_DigestCarriesItsNotificationID(t *testing.T) {
	store := &fakeStore{
		claimed:            []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}, {SubscriptionID: 1, JobID: 11}},
		delivery:           deliverySubscription(),
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}, 11: {ID: 11, Title: "B", PublicSlug: "b"}},
		nextNotificationID: 42,
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("digests sent = %d, want 1", len(notifier.sent))
	}
	if notifier.sent[0].NotificationID != 42 {
		t.Errorf("NotificationID = %d, want 42 — the digest must be recorded before it is sent", notifier.sent[0].NotificationID)
	}
	if len(store.deletedNotifications) != 0 {
		t.Errorf("deleted = %v, want none on a successful delivery", store.deletedNotifications)
	}
}

func TestDeliver_FailedSendWithdrawsItsNotification(t *testing.T) {
	store := &fakeStore{
		claimed:            []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery:           deliverySubscription(),
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{err: errors.New("telegram down")}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
	if len(store.deletedNotifications) != 1 || store.deletedNotifications[0] != 42 {
		t.Errorf("deleted = %v, want [42] — an undelivered digest must not sit in the history", store.deletedNotifications)
	}
	if len(store.notified) != 0 {
		t.Errorf("notified = %d, want 0 (a failed send leaves matches pending)", len(store.notified))
	}
	if len(store.failures) != 1 {
		t.Errorf("failures = %d, want 1", len(store.failures))
	}
}

func TestDeliver_SoftSkippedChannelWithdrawsItsNotification(t *testing.T) {
	store := &fakeStore{
		claimed:            []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery:           deliverySubscription(),
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{err: ErrChannelNotConfigured}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.SoftSkips != 1 {
		t.Errorf("SoftSkips = %d, want 1", stats.SoftSkips)
	}
	if len(store.deletedNotifications) != 1 || store.deletedNotifications[0] != 42 {
		t.Errorf("deleted = %v, want [42] — nothing was delivered, so nothing belongs in the history", store.deletedNotifications)
	}
}

func TestDeliver_RecordFailureSendsWithoutANotificationID(t *testing.T) {
	store := &fakeStore{
		claimed:               []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery:              deliverySubscription(),
		digestJobs:            map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		recordNotificationErr: errors.New("insert failed"),
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("digests sent = %d, want 1 (a recording failure must not block the send)", len(notifier.sent))
	}
	if notifier.sent[0].NotificationID != 0 {
		t.Errorf("NotificationID = %d, want 0", notifier.sent[0].NotificationID)
	}
	if len(store.deletedNotifications) != 0 {
		t.Errorf("deleted = %v, want none — there is no row to withdraw", store.deletedNotifications)
	}
}

func TestDeliver_DigestRecordsEveryMatchedJob(t *testing.T) {
	claimed := make([]db.ClaimSubscriptionMatchesRow, 0, 67)
	jobs := make(map[int64]db.GetJobsForDigestRow, 67)
	for i := range int64(67) {
		claimed = append(claimed, db.ClaimSubscriptionMatchesRow{SubscriptionID: 1, JobID: i})
		jobs[i] = db.GetJobsForDigestRow{ID: i, Title: "Job", PublicSlug: "job"}
	}
	store := &fakeStore{claimed: claimed, delivery: deliverySubscription(), digestJobs: jobs}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := notifier.sent[0]
	if d.Total != 67 || len(d.Jobs) != 67 {
		t.Errorf("digest Total=%d Jobs=%d, want 67/67", d.Total, len(d.Jobs))
	}
	if len(d.Listed()) != ListLimit {
		t.Errorf("Listed = %d, want %d", len(d.Listed()), ListLimit)
	}

	var recorded []SnapshotJob
	if err := json.Unmarshal(store.recordedNotifications[0].Jobs, &recorded); err != nil {
		t.Fatalf("snapshot did not unmarshal: %v", err)
	}
	if len(recorded) != 67 {
		t.Errorf("recorded snapshot carries %d jobs, want 67", len(recorded))
	}
}

// A pass can claim more matches for one subscription than a digest can carry.
// The overflow must go back to pending — stamping it notified would drop those
// postings from the alert forever, listed in no message and in no snapshot.
func TestDeliver_OverflowBeyondSnapshotCapIsDeferredNotDropped(t *testing.T) {
	claimed := make([]db.ClaimSubscriptionMatchesRow, 0, 5)
	jobs := make(map[int64]db.GetJobsForDigestRow, 5)
	for i := range int64(5) {
		claimed = append(claimed, db.ClaimSubscriptionMatchesRow{SubscriptionID: 1, JobID: i})
		jobs[i] = db.GetJobsForDigestRow{ID: i, Title: "Job", PublicSlug: "job"}
	}
	store := &fakeStore{claimed: claimed, delivery: deliverySubscription(), digestJobs: jobs}
	notifier := &fakeNotifier{}
	cfg := DefaultConfig()
	cfg.SnapshotCap = 3
	r := New(store, &fakeSearcher{}, notifier, cfg)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := notifier.sent[0].Total; got != 3 {
		t.Errorf("digest Total = %d, want 3 — a digest announces only what it carries", got)
	}
	if len(store.notified) != 1 || len(store.notified[0].JobIds) != 3 {
		t.Fatalf("notified = %+v, want exactly the 3 delivered jobs", store.notified)
	}
	if len(store.released) != 1 || len(store.released[0].JobIds) != 2 {
		t.Fatalf("released = %+v, want the 2 undelivered jobs back in the pending queue", store.released)
	}
	for _, id := range store.notified[0].JobIds {
		for _, rel := range store.released[0].JobIds {
			if id == rel {
				t.Errorf("job %d was both notified and released", id)
			}
		}
	}
}

// A claimed id whose job row is gone (pruned) must still be stamped notified,
// or it is re-claimed on every pass forever.
func TestDeliver_ClaimedIDWithNoJobRowIsStillNotified(t *testing.T) {
	store := &fakeStore{
		claimed:    []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}, {SubscriptionID: 1, JobID: 99}},
		delivery:   deliverySubscription(),
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
	}
	cfg := DefaultConfig()
	cfg.SnapshotCap = 1
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, cfg)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.notified) != 1 || len(store.notified[0].JobIds) != 2 {
		t.Errorf("notified = %+v, want both ids (the pruned one included)", store.notified)
	}
}

// The bug these guard: a subscriber who blocked the Telegram bot answered every
// send with 403. The digest counted a delivery failure, the run exited non-zero,
// and freehire-notify.service sat in `failed` — one stranger muting a bot turning
// a worker red every five minutes, forever, because nothing ever concluded that
// the chat was not coming back.
func TestDeliver_RecipientGoneUnlinksTelegramAndSoftSkips(t *testing.T) {
	store := &fakeStore{
		claimed:            []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery:           deliverySubscription(),
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{err: ErrRecipientGone}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.SoftSkips != 1 || stats.Failed != 0 {
		t.Errorf("SoftSkips = %d, Failed = %d, want 1 and 0 — a chat that will never accept us is not a delivery failure to retry", stats.SoftSkips, stats.Failed)
	}
	if len(store.unlinkedTelegram) != 1 || store.unlinkedTelegram[0] != 42 {
		t.Errorf("unlinkedTelegram = %v, want [42] — the link is what has to go, so every later send soft-skips instead of re-failing", store.unlinkedTelegram)
	}
	if len(store.failures) != 0 {
		t.Errorf("failures = %v, want none — burning attempts toward the dead-letter limit cannot help here", store.failures)
	}
	// The matches go back to pending rather than being stamped notified: nobody
	// received them, and they are owed over whatever channel comes next.
	if len(store.released) != 1 {
		t.Errorf("released = %v, want the claim released", store.released)
	}
	if len(store.notified) != 0 {
		t.Errorf("notified = %v, want none — the digest was never delivered", store.notified)
	}
	if len(store.deletedNotifications) != 1 || store.deletedNotifications[0] != 42 {
		t.Errorf("deleted = %v, want [42] — nothing was delivered, so nothing belongs in the history", store.deletedNotifications)
	}
}

// The unlink is best-effort: it changes what the NEXT pass does, and this pass
// soft-skips either way. A store that refuses must not turn the skip back into a
// failure, or the red-worker loop returns by another door.
func TestDeliver_RecipientGoneSoftSkipsEvenIfUnlinkFails(t *testing.T) {
	store := &fakeStore{
		claimed:           []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery:          deliverySubscription(),
		digestJobs:        map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		unlinkTelegramErr: errors.New("boom"),
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{err: ErrRecipientGone}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.SoftSkips != 1 || stats.Failed != 0 {
		t.Errorf("SoftSkips = %d, Failed = %d, want 1 and 0", stats.SoftSkips, stats.Failed)
	}
}
