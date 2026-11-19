package notify

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

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
func TestMatch_ForwardsEachSubscriptionsQueryRevision(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, Query: "seniority=senior", QueryRevision: 3, StartAt: ts(base)},
			{ID: 2, Query: "seniority=senior", QueryRevision: 5, StartAt: ts(base)},
		},
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hit(100, base.Add(time.Hour))}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recordedRevisions) != 2 || store.recordedRevisions[1] != 3 || store.recordedRevisions[2] != 5 {
		t.Errorf("revisions forwarded per subscription = %v, want {1:3, 2:5}", store.recordedRevisions)
	}
}

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

func TestMatch_MinSkillMatchIsPerSubscriber(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{
		active: []db.ListActiveSubscriptionsRow{
			{ID: 1, UserID: 1, Query: "seniority=senior&min_skill_match=70", StartAt: ts(base)},
			{ID: 2, UserID: 2, Query: "seniority=senior&min_skill_match=70", StartAt: ts(base)},
			{ID: 3, UserID: 3, Query: "seniority=senior&min_skill_match=70", StartAt: ts(base)}, // no profile
		},
		heldSkills: map[int64][]string{1: {"go"}, 2: {"go", "kafka", "aws"}},
	}
	searcher := &fakeSearcher{results: []search.SearchResult{
		{Hits: []search.JobDocument{hitWithSkills(100, base.Add(time.Hour), "go", "kafka", "aws", "sql")}},
	}}
	r := New(store, searcher, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := map[int64]bool{}
	for _, rec := range store.recorded {
		got[rec.sub] = true
	}
	// 1 holds 25% of the job, 2 holds 75%, and 3 has no skills to compute a percent from.
	if got[1] || !got[2] || !got[3] || len(store.recorded) != 2 {
		t.Errorf("recorded = %+v, want subscriptions 2 and 3", store.recorded)
	}
}

// The expected values mirror the computeClientMatch cases in web/src/lib/jobMatch.test.ts:
// the digest and the job list must agree on what clears a threshold.
func TestMeetsSkillMatch(t *testing.T) {
	cases := []struct {
		name string
		job  []string
		held []string
		min  int
		want bool
	}{
		{"half clears 50", []string{"go", "kafka", "aws", "spark"}, []string{"go", "aws", "react"}, 50, true},
		{"half misses 55", []string{"go", "kafka", "aws", "spark"}, []string{"go", "aws", "react"}, 55, false},
		{"case-insensitive", []string{"Go", "Kafka"}, []string{"go"}, 50, true},
		{"rounds up: 2 of 3 is 67", []string{"a", "b", "c"}, []string{"a", "b"}, 67, true},
		{"no threshold", []string{"go"}, []string{"php"}, 0, true},
		{"job without skills stays", nil, []string{"go"}, 90, true},
		{"reader without skills stays", []string{"go"}, nil, 90, true},
	}
	for _, c := range cases {
		if got := meetsSkillMatch(c.job, c.held, c.min); got != c.want {
			t.Errorf("%s: meetsSkillMatch = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMinSkillMatch(t *testing.T) {
	for raw, want := range map[string]int{
		"70":   70,
		" 70 ": 70, //nolint:gocritic // the padding is the case under test
		"100":  100,
		"1":    1,
		"0":    0,
		"101":  0,
		"-5":   0,
		"abc":  0,
		"":     0,
	} {

		if got := minSkillMatch(url.Values{"min_skill_match": {raw}}); got != want {
			t.Errorf("minSkillMatch(%q) = %d, want %d", raw, got, want)
		}
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
	if len(store.claimParams) != 0 || len(store.released) != 0 {
		t.Errorf("claims = %d, released = %d, want 0/0 (an undeliverable subscription is never leased)", len(store.claimParams), len(store.released))
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
	// Twice: once before the send, and once after it succeeded, because a
	// delivered digest belongs in the history and the first write did not put it
	// there. A store that is down for both leaves no row — that is the cost of a
	// database outage, not of this ordering.
	if len(store.recordedNotifications) != 2 {
		t.Errorf("recorded notifications = %d, want 2 (the pre-send attempt, then the retry a successful send earns)",
			len(store.recordedNotifications))
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
	for _, c := range []string{"", "sms", "Telegram", "e-mail"} {
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

// Webhook is the fourth delivery channel, added alongside Telegram/email/push.
func TestValidChannel_Webhook(t *testing.T) {
	if ChannelWebhook != "webhook" {
		t.Errorf("ChannelWebhook = %q, want %q", ChannelWebhook, "webhook")
	}
	if !ValidChannel(ChannelWebhook) {
		t.Error("ValidChannel(ChannelWebhook) = false, want true")
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

// recipient's webhook case resolves the account's configured destination URL
// rather than a per-subscription value.
func TestRecipient_WebhookEnabledAndConfigured(t *testing.T) {
	info := db.GetSubscriptionForDeliveryRow{
		UserID:         42,
		Channel:        ChannelWebhook,
		WebhookUrl:     pgtype.Text{String: "https://example.com/hook", Valid: true},
		WebhookEnabled: true,
	}
	dest, ok := recipient(info)
	if !ok {
		t.Fatal("recipient: ok = false, want true when a webhook destination is configured and enabled")
	}
	if dest != "https://example.com/hook" {
		t.Errorf("recipient dest = %q, want the webhook URL", dest)
	}
}

func TestRecipient_WebhookDisabledIsNotDeliverable(t *testing.T) {
	info := db.GetSubscriptionForDeliveryRow{
		UserID:         42,
		Channel:        ChannelWebhook,
		WebhookUrl:     pgtype.Text{String: "https://example.com/hook", Valid: true},
		WebhookEnabled: false,
	}
	dest, ok := recipient(info)
	if ok {
		t.Errorf("recipient: ok = true, want false when the webhook destination is disabled")
	}
	if dest != "" {
		t.Errorf("recipient dest = %q, want empty", dest)
	}
}

func TestRecipient_WebhookUnconfiguredIsNotDeliverable(t *testing.T) {
	info := db.GetSubscriptionForDeliveryRow{
		UserID:  42,
		Channel: ChannelWebhook,
	}
	dest, ok := recipient(info)
	if ok {
		t.Errorf("recipient: ok = true, want false when no webhook destination exists")
	}
	if dest != "" {
		t.Errorf("recipient dest = %q, want empty", dest)
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
