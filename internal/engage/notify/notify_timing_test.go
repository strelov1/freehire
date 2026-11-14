package notify

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

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

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("sent = %d, want 0 (deferred during quiet hours)", len(notifier.sent))
	}
	if len(store.claimParams) != 0 || len(store.released) != 0 {
		t.Errorf("claims = %d, released = %d, want 0/0 (a held message is never leased)", len(store.claimParams), len(store.released))
	}
	if stats.Deferred != 1 {
		t.Errorf("deferred = %d, want 1", stats.Deferred)
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
	if len(store.claimParams) != 0 || len(store.released) != 0 {
		t.Errorf("claims = %d, released = %d, want 0/0", len(store.claimParams), len(store.released))
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
// An interval subscription reaching deliverMessage has already passed the claim's
// elapsed-interval gate, so it sends and stamps last_digest_sent_at — the stamp is
// what the NEXT claim measures the interval from.
func TestDeliverOne_IntervalDeliversAndStampsLastSent(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true},
				DigestFrequency: FrequencyInterval, Timezone: pgtype.Text{String: "UTC", Valid: true},
			},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(notifier.sent))
	}
	if len(store.digestSent) != 1 || store.digestSent[0] != 1 {
		t.Errorf("digestSent = %v, want [1] (the next interval is measured from this stamp)", store.digestSent)
	}
}

// Unlike daily, an interval digest names no hour the person chose, so quiet hours
// hold it like an instant one.
func TestDeliverOne_IntervalDeferredDuringQuietHours(t *testing.T) {
	fixedNow := time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC) // inside 22:00-08:00
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true},
				DigestFrequency: FrequencyInterval, Timezone: pgtype.Text{String: "UTC", Valid: true},
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
	if len(notifier.sent) != 0 || len(store.claimParams) != 0 || len(store.released) != 0 {
		t.Errorf("sent=%d claims=%d released=%d, want 0/0/0 (held through quiet hours, never leased)", len(notifier.sent), len(store.claimParams), len(store.released))
	}
	if len(store.digestSent) != 0 {
		t.Errorf("digestSent = %v, want none (nothing was sent)", store.digestSent)
	}
}

// Instant never touches last_digest_sent_at: nothing reads it for that mode.
func TestDeliverOne_InstantDoesNotStampLastSent(t *testing.T) {
	store := &fakeStore{
		claimed: []db.ClaimSubscriptionMatchesRow{{SubscriptionID: 1, JobID: 10}},
		delivery: map[int64]db.GetSubscriptionForDeliveryRow{
			1: {
				ID: 1, Channel: ChannelTelegram, TelegramChatID: pgtype.Int8{Int64: 1, Valid: true},
				DigestFrequency: FrequencyInstant,
			},
		},
		digestJobs: map[int64]db.GetJobsForDigestRow{10: {ID: 10}},
	}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || len(store.digestSent) != 0 {
		t.Errorf("sent=%d digestSent=%v, want 1/none", len(notifier.sent), store.digestSent)
	}
}
