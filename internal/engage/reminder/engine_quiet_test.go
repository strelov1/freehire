package reminder

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestRun_DeferredDuringQuietHours(t *testing.T) {
	chat := int64(555)
	row := actionableRow(1, []string{"telegram"}, &chat, "a@b.c")
	row.Timezone = pgtype.Text{String: "UTC", Valid: true}
	row.QuietHoursStart = pgtype.Time{Microseconds: int64(22 * 3600 * 1e6), Valid: true}
	row.QuietHoursEnd = pgtype.Time{Microseconds: int64(8 * 3600 * 1e6), Valid: true}
	store := &fakeStore{due: []int64{1}, rows: map[int64]db.GetReminderForDeliveryRow{1: row}}
	notifier := &fakeNotifier{}
	r := NewRunner(store, notifier, DefaultConfig())
	r.now = func() time.Time { return time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC) } // inside 22:00-08:00

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("must not send during quiet hours, sent %v", notifier.sent)
	}
	if len(store.released) != 1 {
		t.Errorf("must release the claim, released = %v", store.released)
	}
	if len(store.delivered) != 0 || len(store.cancelled) != 0 {
		t.Errorf("must not deliver or cancel, delivered=%v cancelled=%v", store.delivered, store.cancelled)
	}
	if stats.Deferred != 1 {
		t.Errorf("stats.Deferred = %d, want 1", stats.Deferred)
	}
}

func TestRun_DeliversOutsideQuietHours(t *testing.T) {
	chat := int64(555)
	row := actionableRow(1, []string{"telegram"}, &chat, "a@b.c")
	row.Timezone = pgtype.Text{String: "UTC", Valid: true}
	row.QuietHoursStart = pgtype.Time{Microseconds: int64(22 * 3600 * 1e6), Valid: true}
	row.QuietHoursEnd = pgtype.Time{Microseconds: int64(8 * 3600 * 1e6), Valid: true}
	store := &fakeStore{due: []int64{1}, rows: map[int64]db.GetReminderForDeliveryRow{1: row}}
	notifier := &fakeNotifier{}
	r := NewRunner(store, notifier, DefaultConfig())
	r.now = func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) } // outside 22:00-08:00

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(notifier.sent) != 1 {
		t.Errorf("sent = %v, want 1 delivery outside quiet hours", notifier.sent)
	}
	if stats.Delivered != 1 {
		t.Errorf("stats.Delivered = %d, want 1", stats.Delivered)
	}
}

func TestRun_QuietHoursOffDoesNotDefer(t *testing.T) {
	chat := int64(555)
	row := actionableRow(1, []string{"telegram"}, &chat, "a@b.c") // no quiet hours set
	store := &fakeStore{due: []int64{1}, rows: map[int64]db.GetReminderForDeliveryRow{1: row}}
	stats := run(t, store, &fakeNotifier{})

	if stats.Delivered != 1 || stats.Deferred != 0 {
		t.Errorf("stats = %+v, want Delivered=1 Deferred=0 when quiet hours are unset", stats)
	}
}

// Same failure as the digest worker's: a user who blocked the bot answered every
// telegram send with 403, and each reminder counted that as a delivery failure to
// retry. Retrying cannot reach a closed chat, so the link goes instead — which
// also stops the digest and nudge workers meeting the same wall.
func TestRun_RecipientGoneUnlinksTelegramInsteadOfFailing(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{
		due:  []int64{1},
		rows: map[int64]db.GetReminderForDeliveryRow{1: actionableRow(1, []string{"telegram"}, &chat, "a@b.c")},
	}
	stats := run(t, store, &fakeNotifier{err: ErrRecipientGone})

	if len(store.unlinkedTelegram) != 1 || store.unlinkedTelegram[0] != 42 {
		t.Errorf("unlinkedTelegram = %v, want [42]", store.unlinkedTelegram)
	}
	if len(store.failed) != 0 {
		t.Errorf("failed = %v, want none — a chat that will never accept us is not a failure to retry", store.failed)
	}
	if stats.Failed != 0 {
		t.Errorf("stats.Failed = %d, want 0", stats.Failed)
	}
}

// A reminder is delivered if ANY of its channels lands. A dead Telegram chat must
// not cancel the email that did arrive, nor mark the reminder undelivered.
func TestRun_RecipientGoneOnOneChannelStillDeliversTheOther(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{
		due:  []int64{1},
		rows: map[int64]db.GetReminderForDeliveryRow{1: actionableRow(1, []string{"telegram", "email"}, &chat, "a@b.c")},
	}
	// Telegram is gone; email works.
	notifier := &fakeNotifier{errByChannel: map[string]error{"telegram": ErrRecipientGone}}
	stats := run(t, store, notifier)

	if len(store.delivered) != 1 {
		t.Errorf("delivered = %v, want the reminder delivered over email", store.delivered)
	}
	if stats.Failed != 0 {
		t.Errorf("stats.Failed = %d, want 0", stats.Failed)
	}
	if len(store.unlinkedTelegram) != 1 {
		t.Errorf("unlinkedTelegram = %v, want the dead chat forgotten anyway", store.unlinkedTelegram)
	}
}
