package reminder

import (
	"context"
	"testing"
	"time"

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

	recorded  []db.RecordNotificationParams
	recordErr error
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
func (s *fakeStore) RecordNotification(_ context.Context, arg db.RecordNotificationParams) (int64, error) {
	s.recorded = append(s.recorded, arg)
	return int64(len(s.recorded)), s.recordErr
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
		ID: id, UserID: 42, JobID: id, Channels: channels,
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

func TestRecipient_Push(t *testing.T) {
	tests := []struct {
		name          string
		hasPushDevice bool
		wantDest      string
		wantOK        bool
	}{
		{name: "registered device", hasPushDevice: true, wantDest: "42", wantOK: true},
		{name: "no registered device", hasPushDevice: false, wantDest: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := db.GetReminderForDeliveryRow{UserID: 42, HasPushDevice: tt.hasPushDevice}
			dest, ok := recipient("push", info)
			if dest != tt.wantDest || ok != tt.wantOK {
				t.Errorf("recipient(push, HasPushDevice=%v) = (%q, %v), want (%q, %v)",
					tt.hasPushDevice, dest, ok, tt.wantDest, tt.wantOK)
			}
		})
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

func TestRun_RecordsNotificationOnDelivery(t *testing.T) {
	chat := int64(555)
	row := actionableRow(1, []string{"telegram"}, &chat, "a@b.c")
	store := &fakeStore{due: []int64{1}, rows: map[int64]db.GetReminderForDeliveryRow{1: row}}
	notifier := &fakeNotifier{}
	run(t, store, notifier)

	if len(store.recorded) != 1 {
		t.Fatalf("recorded = %v, want exactly one RecordNotification call", store.recorded)
	}
	rec := store.recorded[0]
	if rec.Kind != "reminder" {
		t.Errorf("Kind = %q, want %q", rec.Kind, "reminder")
	}
	if rec.UserID != row.UserID {
		t.Errorf("UserID = %d, want %d", rec.UserID, row.UserID)
	}
	if !rec.PublicSlug.Valid || rec.PublicSlug.String != row.PublicSlug {
		t.Errorf("PublicSlug = %+v, want a valid slug %q", rec.PublicSlug, row.PublicSlug)
	}
	wantTitle, wantBody := renderReminder(ReminderMessage{JobTitle: row.Title, Company: row.Company, Slug: row.PublicSlug, URL: row.URL})
	if rec.Title != wantTitle {
		t.Errorf("Title = %q, want %q", rec.Title, wantTitle)
	}
	if rec.Body != wantBody {
		t.Errorf("Body = %q, want %q", rec.Body, wantBody)
	}
}

func TestRun_RecordNotificationFailureDoesNotBlockDelivery(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{
		due:       []int64{1},
		rows:      map[int64]db.GetReminderForDeliveryRow{1: actionableRow(1, []string{"telegram"}, &chat, "a@b.c")},
		recordErr: context.DeadlineExceeded,
	}
	notifier := &fakeNotifier{}
	stats := run(t, store, notifier)

	if len(store.delivered) != 1 || store.delivered[0] != 1 {
		t.Errorf("must still mark delivered despite recording failure, delivered = %v", store.delivered)
	}
	if stats.Delivered != 1 {
		t.Errorf("stats.Delivered = %d, want 1 despite recording failure", stats.Delivered)
	}
}

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
