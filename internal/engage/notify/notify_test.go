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
	for raw, want := range map[string]int{"70": 70, " 70 ": 70, "100": 100, "1": 1, "0": 0, "101": 0, "-5": 0, "abc": 0, "": 0} {
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

// deliverySubscription is the minimal Telegram-deliverable subscription the
// ordering tests below all need.
func deliverySubscription() map[int64]db.GetSubscriptionForDeliveryRow {
	return map[int64]db.GetSubscriptionForDeliveryRow{
		1: {ID: 1, UserID: 42, Channel: ChannelTelegram, SavedSearchName: "Go", TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
	}
}

// ErrRecipientGone means different things on different channels: a Telegram bot
// that was blocked, or (per the webhook channel's own mapping, see
// internal/engage/webhooknotify) a destination that answered 410 Gone. deliverMessage
// must dispatch the "forget this recipient" side effect by channel — a webhook's
// 410 must disable the webhook config, never the user's unrelated Telegram link.
func TestDeliver_RecipientGoneOnWebhookDisablesWebhookNotTelegram(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, UserID: 42, Channel: ChannelWebhook, SavedSearchName: "Go",
				WebhookUrl: pgtype.Text{String: "https://example.com/hook", Valid: true}, WebhookEnabled: true,
			},
		},
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{err: ErrRecipientGone}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.SoftSkips != 1 || stats.Failed != 0 {
		t.Errorf("SoftSkips = %d, Failed = %d, want 1 and 0", stats.SoftSkips, stats.Failed)
	}
	if len(store.disabledWebhooks) != 1 || store.disabledWebhooks[0] != 42 {
		t.Errorf("disabledWebhooks = %v, want [42]", store.disabledWebhooks)
	}
	if len(store.unlinkedTelegram) != 0 {
		t.Errorf("unlinkedTelegram = %v, want none — a webhook 410 must not touch the Telegram link", store.unlinkedTelegram)
	}
}

// A successful webhook delivery stamps the destination's last_success_at, so
// the settings page can show "last delivered …" — the counterpart to
// disableWebhook on the failure side.
func TestDeliver_SuccessfulWebhookRecordsDeliverySuccess(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, UserID: 42, Channel: ChannelWebhook, SavedSearchName: "Go",
				WebhookUrl: pgtype.Text{String: "https://example.com/hook", Valid: true}, WebhookEnabled: true,
			},
		},
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recordedWebhookSuccesses) != 1 || store.recordedWebhookSuccesses[0] != 42 {
		t.Errorf("recordedWebhookSuccesses = %v, want [42]", store.recordedWebhookSuccesses)
	}
}

// A webhook send that fails WITHOUT answering 410 Gone (a 404, a 500, a
// timeout) must still count toward the destination's auto-disable threshold
// — otherwise a simply-dead destination (wrong URL, service retired, no 410
// in sight) fails every pass forever instead of being disabled once, the
// freehire incident this mechanism exists to prevent.
func TestDeliver_NonGoneWebhookFailureCountsTowardAutoDisable(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, UserID: 42, Channel: ChannelWebhook, SavedSearchName: "Go",
				WebhookUrl: pgtype.Text{String: "https://example.com/hook", Valid: true}, WebhookEnabled: true,
			},
		},
		digestJobs:           map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID:   42,
		webhookFailureResult: db.RecordWebhookDeliveryFailureRow{ConsecutiveFailures: 1, Enabled: true},
	}
	cfg := DefaultConfig()
	r := New(store, &fakeSearcher{}, &fakeNotifier{err: errors.New("webhooknotify: destination responded 404")}, cfg)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Unlike a 410 (soft-skip), this attempt genuinely failed and must still
	// be counted — only FUTURE passes stop trying once the destination is
	// disabled.
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
	if len(store.recordedWebhookFailures) != 1 {
		t.Fatalf("recordedWebhookFailures = %v, want exactly one call", store.recordedWebhookFailures)
	}
	if got := store.recordedWebhookFailures[0]; got.UserID != 42 || got.MaxFailures != cfg.WebhookMaxConsecutiveFailures {
		t.Errorf("recordedWebhookFailures[0] = %+v, want UserID 42, MaxFailures %d", got, cfg.WebhookMaxConsecutiveFailures)
	}
	// This is the non-410 path: it must never go through disableWebhook,
	// which is reserved for a destination that explicitly answered Gone.
	if len(store.disabledWebhooks) != 0 {
		t.Errorf("disabledWebhooks = %v, want none — a plain 404 is not a 410", store.disabledWebhooks)
	}
}

// A non-webhook channel's failure (Telegram, email, push) must never count
// toward the webhook auto-disable threshold — it belongs only to the webhook
// channel, same restriction as RecordWebhookDeliverySuccess.
func TestDeliver_NonWebhookFailureDoesNotCountTowardAutoDisable(t *testing.T) {
	store := &fakeStore{
		claimed:            []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery:           deliverySubscription(),
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{err: errors.New("telegram: boom")}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recordedWebhookFailures) != 0 {
		t.Errorf("recordedWebhookFailures = %v, want none for a telegram delivery", store.recordedWebhookFailures)
	}
}

// A successful Telegram (or any non-webhook) delivery must not touch the
// webhook success stamp — it belongs only to the webhook channel.
func TestDeliver_SuccessfulTelegramDoesNotRecordWebhookSuccess(t *testing.T) {
	store := &fakeStore{
		claimed:            []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery:           deliverySubscription(),
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recordedWebhookSuccesses) != 0 {
		t.Errorf("recordedWebhookSuccesses = %v, want none for a telegram delivery", store.recordedWebhookSuccesses)
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

// One saved search on two channels is one event: the second channel joins the
// row the first recorded instead of adding an indistinguishable second one, and
// both messages link at the same page (freehire#3020).
func TestDeliver_SecondChannelOfOneSavedSearchJoinsTheSameNotification(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}, {SubscriptionID: 2, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, UserID: 42, SavedSearchID: 7, Channel: ChannelTelegram, SavedSearchName: "My profile", TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
			2: {ID: 2, UserID: 42, SavedSearchID: 7, Channel: ChannelEmail, SavedSearchName: "My profile", AccountEmail: "a@example.test"},
		},
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Delivered != 2 {
		t.Fatalf("Delivered = %d, want 2 — both channels still send", stats.Delivered)
	}
	if len(store.notificationsByDedupKey) != 1 {
		t.Errorf("distinct notification rows = %d, want 1 for one event on two channels", len(store.notificationsByDedupKey))
	}
	if len(notifier.sent) != 2 {
		t.Fatalf("digests sent = %d, want 2", len(notifier.sent))
	}
	if notifier.sent[0].NotificationID != 42 || notifier.sent[1].NotificationID != 42 {
		t.Errorf("NotificationIDs = %d and %d, want both 42 — each channel links at the one recorded event",
			notifier.sent[0].NotificationID, notifier.sent[1].NotificationID)
	}
}

// The channel that JOINED an existing row must not withdraw it when its own
// send fails: the row belongs to the channel that recorded it, whose digest
// went out. Withdrawing here is how a third failing channel would erase the
// history of the two that arrived.
func TestDeliver_JoiningChannelDoesNotWithdrawAnotherChannelsNotification(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}, {SubscriptionID: 2, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, UserID: 42, SavedSearchID: 7, Channel: ChannelTelegram, SavedSearchName: "My profile", TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
			2: {ID: 2, UserID: 42, SavedSearchID: 7, Channel: ChannelEmail, SavedSearchName: "My profile", AccountEmail: "a@example.test"},
		},
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	// Telegram (claimed first) succeeds; email then fails.
	notifier := &failAfterNotifier{succeed: 1, err: errors.New("ses down")}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Delivered != 1 || stats.Failed != 1 {
		t.Fatalf("Delivered = %d, Failed = %d, want 1 and 1", stats.Delivered, stats.Failed)
	}
	if len(store.deletedNotifications) != 0 {
		t.Errorf("deleted = %v, want none — the Telegram digest that DID arrive keeps its history row",
			store.deletedNotifications)
	}
}

// A digest that WAS received must leave a history row, even when the record
// taken before the send came back with nothing. Reaching this needs a second
// notify process — deliver() is one sequential goroutine, so within a pass the
// creator has finished sending and withdrawing before any joiner records — and
// the cost is invisible: the send succeeds, the person gets the message, and
// the history simply does not mention it.
func TestDeliver_DeliveredDigestIsRecordedEvenIfItsRowVanishedFirst(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}, {SubscriptionID: 2, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {ID: 1, UserID: 42, SavedSearchID: 7, Channel: ChannelTelegram, SavedSearchName: "My profile", TelegramChatID: pgtype.Int8{Int64: 555, Valid: true}},
			2: {ID: 2, UserID: 42, SavedSearchID: 7, Channel: ChannelEmail, SavedSearchName: "My profile", AccountEmail: "a@example.test"},
		},
		digestJobs:         map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID: 42,
	}
	// Stand in for the other process withdrawing its row in the window between
	// this channel's conflicting insert and its read.
	store.beforeDedupRead = func(s *fakeStore) {
		s.beforeDedupRead = nil
		clear(s.notificationsByDedupKey)
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Delivered != 2 {
		t.Fatalf("Delivered = %d, want 2", stats.Delivered)
	}
	if len(store.notificationsByDedupKey) != 1 {
		t.Errorf("notification rows = %d, want 1 — the second channel's digest was delivered and must be in the history",
			len(store.notificationsByDedupKey))
	}
}

// The same invariant on the plainer path: the pre-send record simply failed.
// The digest still went out, so the history must still name it.
func TestDeliver_DeliveredDigestIsRecordedAfterAFailedPreSendWrite(t *testing.T) {
	store := &fakeStore{
		claimed:                   []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery:                  deliverySubscription(),
		digestJobs:                map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
		nextNotificationID:        42,
		recordNotificationErrOnce: errors.New("database having a moment"),
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || notifier.sent[0].NotificationID != 0 {
		t.Fatalf("sent = %d digests, first NotificationID = %d; want 1 sent with a zero id (the write failed before it)",
			len(notifier.sent), notifier.sent[0].NotificationID)
	}
	if len(store.notificationsByDedupKey) != 1 {
		t.Errorf("notification rows = %d, want 1 — a delivered digest belongs in the history even when the first write failed",
			len(store.notificationsByDedupKey))
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

// A claimed id whose job row is gone (pruned) must still be stamped notified,
// or it is re-claimed on every pass forever.
func TestDeliver_ClaimedIDWithNoJobRowIsStillNotified(t *testing.T) {
	store := &fakeStore{
		claimed:    []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}, {SubscriptionID: 1, JobID: 99}},
		delivery:   deliverySubscription(),
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10, Title: "A", PublicSlug: "a"}},
	}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

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
