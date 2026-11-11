package notify

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func telegramInfo(id int64, mutate func(*db.GetSubscriptionForDeliveryRow)) db.GetSubscriptionForDeliveryRow {
	info := db.GetSubscriptionForDeliveryRow{
		ID: id, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: id, Valid: true},
		DigestFrequency: FrequencyInstant, Timezone: pgtype.Text{String: "UTC", Valid: true},
	}
	if mutate != nil {
		mutate(&info)
	}
	return info
}

// Only the subscriptions that can be written to now are leased: one in quiet hours and one with
// no linked chat stay out of the claim, so their matches are not stamped and released again on
// every pass, and the due one still goes out.
func TestDeliver_LeasesOnlyDeliverableSubscriptions(t *testing.T) {
	now := time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC)
	quiet := func(i *db.GetSubscriptionForDeliveryRow) {
		i.QuietHoursStart, i.QuietHoursEnd = pgTime(22, 0), pgTime(8, 0)
	}
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{
			{SubscriptionID: 1, JobID: 10}, {SubscriptionID: 2, JobID: 20}, {SubscriptionID: 3, JobID: 30},
		},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: telegramInfo(1, quiet),
			2: telegramInfo(2, func(i *db.GetSubscriptionForDeliveryRow) { i.TelegramChatID = pgtype.Int8{} }),
			3: telegramInfo(3, nil),
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}, 20: {ID: 20}, 30: {ID: 30}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
	r.now = func() time.Time { return now }

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.claimParams) != 1 || !reflect.DeepEqual(store.claimParams[0].SubscriptionIds, []int64{3}) {
		t.Fatalf("claim params = %+v, want one claim of subscription 3 only", store.claimParams)
	}
	if len(store.released) != 0 {
		t.Errorf("released = %+v, want none: nothing held was ever leased", store.released)
	}
	if len(notifier.sent) != 1 || stats.Delivered != 1 || stats.Deferred != 1 || stats.SoftSkips != 1 {
		t.Errorf("sent=%d delivered=%d deferred=%d softSkips=%d, want 1/1/1/1", len(notifier.sent), stats.Delivered, stats.Deferred, stats.SoftSkips)
	}
}

// With nothing deliverable the claim statement is not issued at all.
func TestDeliver_NothingDeliverableIssuesNoClaim(t *testing.T) {
	store := &fakeStore{
		claimed:  []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{1: telegramInfo(1, func(i *db.GetSubscriptionForDeliveryRow) { i.TelegramChatID = pgtype.Int8{} })},
	}
	stats, err := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig()).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.claimParams) != 0 || stats.SoftSkips != 1 {
		t.Errorf("claims=%d softSkips=%d, want 0 and 1", len(store.claimParams), stats.SoftSkips)
	}
}

func TestDeliver_LastSendIsScopedToTheMessage(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name                        string
		firstUser, secondUser       int64
		firstChannel, secondChannel string
	}{
		{"different users", 1, 2, ChannelTelegram, ChannelTelegram},
		{"different channels", 1, 1, ChannelTelegram, ChannelEmail},
		{"different webhooks", 1, 1, ChannelWebhook, ChannelWebhook},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := func(id, userID int64, channel string) db.GetSubscriptionForDeliveryRow {
				return db.GetSubscriptionForDeliveryRow{
					ID: id, UserID: userID, Channel: channel,
					DigestFrequency: FrequencyDaily, DigestTime: pgTime(9, 0),
					Timezone:       pgtype.Text{String: "UTC", Valid: true},
					TelegramChatID: pgtype.Int8{Int64: userID, Valid: true},
					AccountEmail:   "recipient@example.test",
					WebhookUrl:     pgtype.Text{String: "https://example.test/webhook", Valid: true}, WebhookEnabled: true,
				}
			}
			first := info(1, tc.firstUser, tc.firstChannel)
			first.LastDigestSentAt = pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}
			store := &fakeStore{
				claimed: []db.ClaimSubscriptionMatchesRow{
					{SubscriptionID: 1, JobID: 10, UserID: tc.firstUser, Channel: tc.firstChannel},
					{SubscriptionID: 2, JobID: 20, UserID: tc.secondUser, Channel: tc.secondChannel},
				},
				delivery:   map[int64]db.GetSubscriptionForDeliveryRow{1: first, 2: info(2, tc.secondUser, tc.secondChannel)},
				digestJobs: map[int64]db.GetJobsForDigestRow{20: {ID: 20}},
			}
			notifier := &fakeNotifier{}
			r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
			r.now = func() time.Time { return now }
			stats, err := r.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(store.claimParams) != 1 || !reflect.DeepEqual(store.claimParams[0].SubscriptionIds, []int64{2}) {
				t.Fatalf("claim params = %+v, want only subscription 2", store.claimParams)
			}
			if stats.Delivered != 1 || len(notifier.sent) != 1 {
				t.Fatalf("delivered=%d sent=%d, want 1/1", stats.Delivered, len(notifier.sent))
			}
		})
	}
}

// The recheck after the lease stays: quiet hours began, or the chat was unlinked, between the
// listing and the claim. The lease is then released, as before.
func TestDeliver_RechecksAfterTheLease(t *testing.T) {
	now := time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC)
	for name, changed := range map[string]func(*db.GetSubscriptionForDeliveryRow){
		"quiet hours began": func(i *db.GetSubscriptionForDeliveryRow) {
			i.QuietHoursStart, i.QuietHoursEnd = pgTime(22, 0), pgTime(8, 0)
		},
		"chat unlinked": func(i *db.GetSubscriptionForDeliveryRow) { i.TelegramChatID = pgtype.Int8{} },
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{
				claimed:    []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
				listed:     map[int64]db.GetSubscriptionForDeliveryRow{1: telegramInfo(1, nil)},
				delivery:   map[int64]db.GetSubscriptionForDeliveryRow{1: telegramInfo(1, changed)},
				digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
			}
			notifier := &fakeNotifier{}
			r := New(store, &fakeSearcher{}, notifier, DefaultConfig())
			r.now = func() time.Time { return now }

			if _, err := r.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(notifier.sent) != 0 || len(store.released) != 1 || len(store.failures) != 0 {
				t.Errorf("sent=%d released=%d failures=%d, want 0/1/0", len(notifier.sent), len(store.released), len(store.failures))
			}
		})
	}
}

// Timing is the account's own local time, DST included, and the person's last send counts
// across all their alerts: one alert already mailed today holds a sibling that has no stamp.
func TestDeliveryHold(t *testing.T) {
	ny := pgtype.Text{String: "America/New_York", Valid: true}
	daily := func(i *db.GetSubscriptionForDeliveryRow) {
		i.DigestFrequency, i.DigestTime, i.Timezone = FrequencyDaily, pgTime(9, 0), ny
	}
	// 2026-03-08 is the US spring-forward day: 09:00 local is 13:00 UTC, not 14:00.
	before := time.Date(2026, 3, 8, 12, 59, 0, 0, time.UTC)
	after := time.Date(2026, 3, 8, 13, 1, 0, 0, time.UTC)
	yesterday := pgtype.Timestamptz{Time: time.Date(2026, 3, 7, 14, 0, 0, 0, time.UTC), Valid: true}
	today := pgtype.Timestamptz{Time: time.Date(2026, 3, 8, 13, 0, 30, 0, time.UTC), Valid: true}

	cases := []struct {
		name string
		now  time.Time
		info db.GetSubscriptionForDeliveryRow
		last pgtype.Timestamptz
		want hold
	}{
		{"daily before the local digest time", before, telegramInfo(1, daily), yesterday, holdDeferred},
		{"daily at the local digest time across DST", after, telegramInfo(1, daily), yesterday, holdNone},
		{"daily already sent today, by a sibling alert", after, telegramInfo(1, daily), today, holdDeferred},
		{"daily ignores quiet hours", after, telegramInfo(1, func(i *db.GetSubscriptionForDeliveryRow) {
			daily(i)
			i.QuietHoursStart, i.QuietHoursEnd = pgTime(0, 0), pgTime(23, 59)
		}), yesterday, holdNone},
		{"interval holds in quiet hours", after, telegramInfo(1, func(i *db.GetSubscriptionForDeliveryRow) {
			i.DigestFrequency, i.Timezone = FrequencyInterval, ny
			i.QuietHoursStart, i.QuietHoursEnd = pgTime(9, 0), pgTime(10, 0)
		}), pgtype.Timestamptz{}, holdDeferred},
		{"no recipient", after, telegramInfo(1, func(i *db.GetSubscriptionForDeliveryRow) { i.TelegramChatID = pgtype.Int8{} }), pgtype.Timestamptz{}, holdNoRecipient},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, got := deliveryHold(c.now, c.info, c.last); got != c.want {
				t.Errorf("deliveryHold = %v, want %v", got, c.want)
			}
		})
	}
}
