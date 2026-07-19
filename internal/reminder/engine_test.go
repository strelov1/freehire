package reminder

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/db"
)

// fakeStore is a DB-free reminder.Store. It serves canned delivery rows keyed by
// id and records which finalizers the engine invoked.
type fakeStore struct {
	due      []int64
	rows     map[int64]db.GetReminderForDeliveryRow
	claimErr error

	delivered []int64
	cancelled []int64
	failed    []int64
	released  []int64
}

func (s *fakeStore) ClaimDueReminders(_ context.Context, _ db.ClaimDueRemindersParams) ([]int64, error) {
	return s.due, s.claimErr
}
func (s *fakeStore) GetReminderForDelivery(_ context.Context, id int64) (db.GetReminderForDeliveryRow, error) {
	return s.rows[id], nil
}
func (s *fakeStore) MarkReminderDelivered(_ context.Context, id int64) (int64, error) {
	s.delivered = append(s.delivered, id)
	return 1, nil
}
func (s *fakeStore) CancelReminderAtFire(_ context.Context, id int64) (int64, error) {
	s.cancelled = append(s.cancelled, id)
	return 1, nil
}
func (s *fakeStore) RecordReminderDeliveryFailure(_ context.Context, arg db.RecordReminderDeliveryFailureParams) error {
	s.failed = append(s.failed, arg.ID)
	return nil
}
func (s *fakeStore) ReleaseReminderClaim(_ context.Context, id int64) error {
	s.released = append(s.released, id)
	return nil
}

// fakeNotifier records deliveries and can be told to fail.
type fakeNotifier struct {
	sent []string // "channel:dest"
	err  error
}

func (n *fakeNotifier) Send(_ context.Context, channel, dest string, _ ReminderMessage) error {
	if n.err != nil {
		return n.err
	}
	n.sent = append(n.sent, channel+":"+dest)
	return nil
}

func actionableRow(id int64, channels []string, chatID *int64, email string) db.GetReminderForDeliveryRow {
	row := db.GetReminderForDeliveryRow{
		ID: id, JobID: id, Channels: channels,
		Title: "Go Dev", Company: "Acme", PublicSlug: "go-dev-acme", URL: "https://ats/x",
		JobOpen: true, StillActionable: true, AccountEmail: email,
	}
	if chatID != nil {
		row.TelegramChatID = pgtype.Int8{Int64: *chatID, Valid: true}
	}
	return row
}

func run(t *testing.T, store *fakeStore, notifier Notifier) Stats {
	t.Helper()
	r := NewRunner(store, notifier, DefaultConfig())
	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return stats
}

func TestRun_DeliversDueReminderOnce(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{
		due:  []int64{1},
		rows: map[int64]db.GetReminderForDeliveryRow{1: actionableRow(1, []string{"telegram"}, &chat, "a@b.c")},
	}
	notifier := &fakeNotifier{}
	stats := run(t, store, notifier)

	if len(notifier.sent) != 1 || notifier.sent[0] != "telegram:555" {
		t.Errorf("sent = %v, want [telegram:555]", notifier.sent)
	}
	if len(store.delivered) != 1 || store.delivered[0] != 1 {
		t.Errorf("delivered = %v, want [1]", store.delivered)
	}
	if stats.Delivered != 1 {
		t.Errorf("stats.Delivered = %d, want 1", stats.Delivered)
	}
}

func TestRun_SoftSkipsWhenNoDestination(t *testing.T) {
	// Channel is telegram but the user has no linked chat → nothing to send.
	store := &fakeStore{
		due:  []int64{1},
		rows: map[int64]db.GetReminderForDeliveryRow{1: actionableRow(1, []string{"telegram"}, nil, "a@b.c")},
	}
	notifier := &fakeNotifier{}
	stats := run(t, store, notifier)

	if len(notifier.sent) != 0 {
		t.Errorf("must not send with no destination, sent %v", notifier.sent)
	}
	if len(store.released) != 1 {
		t.Errorf("must release the claim, released = %v", store.released)
	}
	if len(store.delivered) != 0 {
		t.Errorf("must not mark delivered, delivered = %v", store.delivered)
	}
	if stats.SoftSkips != 1 {
		t.Errorf("stats.SoftSkips = %d, want 1", stats.SoftSkips)
	}
}

func TestRun_CancelsWhenNoLongerActionable(t *testing.T) {
	chat := int64(555)
	row := actionableRow(1, []string{"telegram"}, &chat, "a@b.c")
	row.StillActionable = false // user applied or unsaved before the fire
	store := &fakeStore{due: []int64{1}, rows: map[int64]db.GetReminderForDeliveryRow{1: row}}
	notifier := &fakeNotifier{}
	stats := run(t, store, notifier)

	if len(notifier.sent) != 0 {
		t.Errorf("must not send a stale reminder, sent %v", notifier.sent)
	}
	if len(store.cancelled) != 1 || store.cancelled[0] != 1 {
		t.Errorf("must cancel at fire, cancelled = %v", store.cancelled)
	}
	if stats.Cancelled != 1 {
		t.Errorf("stats.Cancelled = %d, want 1", stats.Cancelled)
	}
}

func TestRun_CancelsWhenJobClosed(t *testing.T) {
	chat := int64(555)
	row := actionableRow(1, []string{"telegram"}, &chat, "a@b.c")
	row.JobOpen = false // job closed before the fire
	store := &fakeStore{due: []int64{1}, rows: map[int64]db.GetReminderForDeliveryRow{1: row}}
	notifier := &fakeNotifier{}
	run(t, store, notifier)

	if len(store.cancelled) != 1 {
		t.Errorf("closed job must be cancelled, cancelled = %v", store.cancelled)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("must not send for a closed job, sent %v", notifier.sent)
	}
}

func TestRun_RecordsFailureOnDeliveryError(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{
		due:  []int64{1},
		rows: map[int64]db.GetReminderForDeliveryRow{1: actionableRow(1, []string{"telegram"}, &chat, "a@b.c")},
	}
	notifier := &fakeNotifier{err: context.DeadlineExceeded}
	stats := run(t, store, notifier)

	if len(store.failed) != 1 || store.failed[0] != 1 {
		t.Errorf("must record failure, failed = %v", store.failed)
	}
	if len(store.delivered) != 0 {
		t.Errorf("must not mark delivered on error, delivered = %v", store.delivered)
	}
	if stats.Failed != 1 {
		t.Errorf("stats.Failed = %d, want 1", stats.Failed)
	}
}

func TestRun_DeliversEmailWhenTelegramMissing(t *testing.T) {
	// Both channels configured; only email has a destination.
	store := &fakeStore{
		due:  []int64{1},
		rows: map[int64]db.GetReminderForDeliveryRow{1: actionableRow(1, []string{"telegram", "email"}, nil, "a@b.c")},
	}
	notifier := &fakeNotifier{}
	run(t, store, notifier)

	if len(notifier.sent) != 1 || notifier.sent[0] != "email:a@b.c" {
		t.Errorf("sent = %v, want [email:a@b.c]", notifier.sent)
	}
	if len(store.delivered) != 1 {
		t.Errorf("a partial-channel delivery still counts as delivered, delivered = %v", store.delivered)
	}
}
