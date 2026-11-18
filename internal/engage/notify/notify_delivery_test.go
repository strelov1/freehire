package notify

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

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
