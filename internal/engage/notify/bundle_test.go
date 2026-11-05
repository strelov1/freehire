package notify

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// bundleStore is one person (user 42) with three email alerts and a webhook alert,
// plus a second person (user 43) with one email alert.
func bundleStore() *fakeStore {
	email := func(id, user int64, name string) db.GetSubscriptionForDeliveryRow {
		return db.GetSubscriptionForDeliveryRow{ID: id, UserID: user, SavedSearchID: id, Channel: ChannelEmail, SavedSearchName: name, AccountEmail: "a@example.test"}
	}
	return &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{
			{SubscriptionID: 1, JobID: 10, UserID: 42, Channel: ChannelEmail},
			{SubscriptionID: 2, JobID: 11, UserID: 42, Channel: ChannelEmail},
			{SubscriptionID: 2, JobID: 12, UserID: 42, Channel: ChannelEmail},
			{SubscriptionID: 3, JobID: 13, UserID: 42, Channel: ChannelEmail},
			{SubscriptionID: 4, JobID: 14, UserID: 43, Channel: ChannelEmail},
			{SubscriptionID: 5, JobID: 15, UserID: 42, Channel: ChannelWebhook},
			{SubscriptionID: 6, JobID: 16, UserID: 42, Channel: ChannelWebhook},
		},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: email(1, 42, "Go"),
			2: email(2, 42, "Rust"),
			3: email(3, 42, "My profile"),
			4: email(4, 43, "Python"),
			5: {ID: 5, UserID: 42, SavedSearchID: 5, Channel: ChannelWebhook, SavedSearchName: "Hook A", WebhookEnabled: true, WebhookUrl: pgtype.Text{String: "https://hook.test", Valid: true}},
			6: {ID: 6, UserID: 42, SavedSearchID: 6, Channel: ChannelWebhook, SavedSearchName: "Hook B", WebhookEnabled: true, WebhookUrl: pgtype.Text{String: "https://hook.test", Valid: true}},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{
			10: {ID: 10, Title: "A"}, 11: {ID: 11, Title: "B"}, 12: {ID: 12, Title: "C"},
			13: {ID: 13, Title: "D"}, 14: {ID: 14, Title: "E"}, 15: {ID: 15, Title: "F"}, 16: {ID: 16, Title: "G"},
		},
	}
}

func digestNames(m Message) []string {
	names := make([]string, len(m.Digests))
	for i, d := range m.Digests {
		names[i] = d.SavedSearchName
	}
	return names
}

func TestDeliver_OneMessagePerPersonAndChannel(t *testing.T) {
	store := bundleStore()
	notifier := &fakeNotifier{}
	stats, err := New(store, &fakeSearcher{}, notifier, DefaultConfig()).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// User 42's three email alerts are one mail; user 43 gets their own; each webhook
	// alert stays its own payload.
	if len(notifier.messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(notifier.messages))
	}
	if got := digestNames(notifier.messages[0]); !slices.Equal(got, []string{"Go", "Rust", "My profile"}) {
		t.Errorf("user 42 email digests = %v, want [Go Rust My profile]", got)
	}
	if m := notifier.messages[0]; m.UserID != 42 || m.Total() != 4 {
		t.Errorf("user 42 email message = user %d total %d, want user 42 total 4", m.UserID, m.Total())
	}
	if got := digestNames(notifier.messages[1]); !slices.Equal(got, []string{"Python"}) {
		t.Errorf("user 43 digests = %v, want [Python]", got)
	}
	for _, m := range notifier.messages[2:] {
		if len(m.Digests) != 1 {
			t.Errorf("webhook message carries %d digests, want 1", len(m.Digests))
		}
	}
	if stats.Delivered != 4 {
		t.Errorf("delivered = %d, want 4 (messages, not digests)", stats.Delivered)
	}

	// Every subscription the mail carried is finalized and stamped, which is what the
	// per-person interval gate reads.
	if len(store.notified) != 6 {
		t.Errorf("mark-notified calls = %d, want 6 (one per subscription)", len(store.notified))
	}
	slices.Sort(store.digestSent)
	if !slices.Equal(store.digestSent, []int64{1, 2, 3, 4, 5, 6}) {
		t.Errorf("digest-sent stamps = %v, want every subscription", store.digestSent)
	}
	// The notification centre keeps one row per saved search, so each section links
	// to its own page.
	if len(store.recordedNotifications) != 6 {
		t.Errorf("notification rows = %d, want 6", len(store.recordedNotifications))
	}
}

func TestDeliver_FailedBundleFailsEveryDigestInIt(t *testing.T) {
	store := bundleStore()
	store.claimed = store.claimed[:4] // user 42's email alerts only
	notifier := &fakeNotifier{err: errors.New("ses down")}
	stats, _ := New(store, &fakeSearcher{}, notifier, DefaultConfig()).Run(context.Background())

	if len(notifier.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(notifier.messages))
	}
	if stats.Failed != 1 {
		t.Errorf("failed = %d, want 1 (one message)", stats.Failed)
	}
	if len(store.failures) != 3 {
		t.Errorf("failure records = %d, want 3 (one per subscription in the mail)", len(store.failures))
	}
	if len(store.notified) != 0 || len(store.digestSent) != 0 {
		t.Errorf("a failed mail marked %d notified and %d sent, want none", len(store.notified), len(store.digestSent))
	}
	if len(store.deletedNotifications) != 3 {
		t.Errorf("withdrawn rows = %d, want 3", len(store.deletedNotifications))
	}
}

func TestDeliver_UnreadableSubscriptionLeavesTheRestOfTheBundle(t *testing.T) {
	store := bundleStore()
	store.claimed = store.claimed[:4]
	delete(store.delivery, 2)
	notifier := &fakeNotifier{}
	reads := &missingDeliveryStore{fakeStore: store, missing: 2}
	stats, _ := New(reads, &fakeSearcher{}, notifier, DefaultConfig()).Run(context.Background())

	if len(notifier.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(notifier.messages))
	}
	if got := digestNames(notifier.messages[0]); !slices.Equal(got, []string{"Go", "My profile"}) {
		t.Errorf("digests = %v, want [Go My profile]", got)
	}
	if stats.Failed != 1 || stats.Delivered != 1 {
		t.Errorf("failed=%d delivered=%d, want 1 and 1", stats.Failed, stats.Delivered)
	}
	if len(store.released) != 1 || store.released[0].SubscriptionID != 2 {
		t.Errorf("released = %+v, want subscription 2 only", store.released)
	}
}

// missingDeliveryStore fails the delivery read of one subscription.
type missingDeliveryStore struct {
	*fakeStore
	missing int64
}

func (s *missingDeliveryStore) GetSubscriptionForDelivery(ctx context.Context, id int64) (db.GetSubscriptionForDeliveryRow, error) {
	if id == s.missing {
		return db.GetSubscriptionForDeliveryRow{}, errors.New("read failed")
	}
	return s.fakeStore.GetSubscriptionForDelivery(ctx, id)
}

// A `daily` person mailed this morning by alert A must not get a second digest when
// alert B, never sent, matches in the afternoon: the claim's person-level stamp
// decides, not B's own empty one.
func TestDeliver_DailyNewAlertWaitsForTomorrow(t *testing.T) {
	sentThisMorning := pgtype.Timestamptz{Time: time.Date(2026, 8, 14, 9, 5, 0, 0, time.UTC), Valid: true}
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{
			{SubscriptionID: 2, JobID: 10, UserID: 42, Channel: ChannelEmail, PersonLastSentAt: sentThisMorning},
		},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			2: {
				ID: 2, UserID: 42, Channel: ChannelEmail, AccountEmail: "a@example.test", SavedSearchName: "B",
				DigestFrequency: FrequencyDaily, DigestTime: pgTime(9, 0), Timezone: pgtype.Text{String: "UTC", Valid: true},
			},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
	r.now = func() time.Time { return time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC) }

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.messages) != 0 || stats.Deferred != 1 {
		t.Errorf("messages=%d deferred=%d, want 0 and 1 (already mailed today)", len(notifier.messages), stats.Deferred)
	}
}

// The claim returns rows in no defined order; a message's sections are by subscription
// id regardless, so a person's mail keeps its shape from one hour to the next.
func TestDeliver_SectionsAreInSubscriptionOrder(t *testing.T) {
	store := bundleStore()
	store.claimed = []db.ClaimSubscriptionMatchesRow{store.claimed[3], store.claimed[0], store.claimed[1]} // 3, 1, 2
	notifier := &fakeNotifier{}
	if _, err := New(store, &fakeSearcher{}, notifier, DefaultConfig()).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := digestNames(notifier.messages[0]); !slices.Equal(got, []string{"Go", "Rust", "My profile"}) {
		t.Errorf("sections = %v, want [Go Rust My profile]", got)
	}
}
