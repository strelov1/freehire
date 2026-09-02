package reminder

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeStore is a DB-free reminder.Store. It serves canned delivery rows keyed by
// id and records which finalizers the engine invoked.
type fakeStore struct {
	// unlinkedTelegram records the user ids a delivery forgot the Telegram chat for.
	unlinkedTelegram []int64

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
func (s *fakeStore) DeleteTelegramLink(_ context.Context, userID int64) (int64, error) {
	s.unlinkedTelegram = append(s.unlinkedTelegram, userID)
	return 1, nil
}
func (s *fakeStore) RecordNotification(_ context.Context, arg db.RecordNotificationParams) (int64, error) {
	s.recorded = append(s.recorded, arg)
	return int64(len(s.recorded)), s.recordErr
}

// fakeNotifier records deliveries and can be told to fail.
type fakeNotifier struct {
	sent []string // "channel:dest"
	// groups is the message list of each recorded send, so a test can assert what
	// a delivery carried and not only that it happened.
	groups [][]ReminderMessage
	err    error
	// errByChannel fails only the named channels, so a test can model one channel
	// dying while another lands — which is the whole point of a multi-channel
	// reminder and the case a blanket err cannot express.
	errByChannel map[string]error
}

func (n *fakeNotifier) Send(_ context.Context, channel, dest string, ms []ReminderMessage) error {
	if err, ok := n.errByChannel[channel]; ok {
		return err
	}
	if n.err != nil {
		return n.err
	}
	n.sent = append(n.sent, channel+":"+dest)
	n.groups = append(n.groups, ms)
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

// ownedBy is actionableRow's multi-account variant: the same deliverable row, but
// belonging to userID and naming its own job, so a test can put two accounts in one
// pass.
func ownedBy(id, userID int64, channels []string, email string) db.GetReminderForDeliveryRow {
	row := actionableRow(id, channels, nil, email)
	row.UserID = userID
	row.Title = "Job " + strconv.FormatInt(id, 10)
	row.PublicSlug = "job-" + strconv.FormatInt(id, 10)
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

// The change's whole point: a day's saves become one message, not one each.
func TestRun_GroupsOneAccountsRemindersIntoOneSend(t *testing.T) {
	store := &fakeStore{
		due: []int64{1, 2, 3},
		rows: map[int64]db.GetReminderForDeliveryRow{
			1: ownedBy(1, 42, []string{"email"}, "a@b.c"),
			2: ownedBy(2, 42, []string{"email"}, "a@b.c"),
			3: ownedBy(3, 42, []string{"email"}, "a@b.c"),
		},
	}
	notifier := &fakeNotifier{}
	stats := run(t, store, notifier)

	if len(notifier.sent) != 1 {
		t.Fatalf("sent = %v, want exactly one message for one account", notifier.sent)
	}
	if got := notifier.groups[0]; len(got) != 3 {
		t.Errorf("the message carried %d jobs, want 3", len(got))
	}
	if len(store.delivered) != 3 {
		t.Errorf("delivered = %v, want all three reminders stamped", store.delivered)
	}
	if stats.Delivered != 3 {
		t.Errorf("stats.Delivered = %d, want 3 — the counter stays per reminder", stats.Delivered)
	}
}

// Grouping must not leak one account's saved jobs into another's message.
func TestRun_DifferentAccountsGetTheirOwnSend(t *testing.T) {
	store := &fakeStore{
		due: []int64{1, 2},
		rows: map[int64]db.GetReminderForDeliveryRow{
			1: ownedBy(1, 42, []string{"email"}, "a@b.c"),
			2: ownedBy(2, 77, []string{"email"}, "x@y.z"),
		},
	}
	notifier := &fakeNotifier{}
	run(t, store, notifier)

	if len(notifier.sent) != 2 {
		t.Fatalf("sent = %v, want one message per account", notifier.sent)
	}
	for i, group := range notifier.groups {
		if len(group) != 1 {
			t.Errorf("group %d carried %d jobs, want 1", i, len(group))
		}
	}
}

// A reminder that lost its intent leaves the batch; the rest still go out together.
func TestRun_CancelledReminderLeavesTheGroup(t *testing.T) {
	stale := ownedBy(2, 42, []string{"email"}, "a@b.c")
	stale.StillActionable = false
	store := &fakeStore{
		due: []int64{1, 2, 3},
		rows: map[int64]db.GetReminderForDeliveryRow{
			1: ownedBy(1, 42, []string{"email"}, "a@b.c"),
			2: stale,
			3: ownedBy(3, 42, []string{"email"}, "a@b.c"),
		},
	}
	notifier := &fakeNotifier{}
	stats := run(t, store, notifier)

	if len(store.cancelled) != 1 || store.cancelled[0] != 2 {
		t.Errorf("cancelled = %v, want [2]", store.cancelled)
	}
	if len(notifier.groups) != 1 || len(notifier.groups[0]) != 2 {
		t.Fatalf("groups = %v, want one message carrying the two survivors", notifier.groups)
	}
	if stats.Delivered != 2 || stats.Cancelled != 1 {
		t.Errorf("stats = %+v, want Delivered=2 Cancelled=1", stats)
	}
}

// The group is the retry unit: a failed send costs every member an attempt, so the
// whole batch comes back on a later pass rather than half of it going missing.
func TestRun_FailedGroupRecordsFailureForEveryMember(t *testing.T) {
	store := &fakeStore{
		due: []int64{1, 2},
		rows: map[int64]db.GetReminderForDeliveryRow{
			1: ownedBy(1, 42, []string{"email"}, "a@b.c"),
			2: ownedBy(2, 42, []string{"email"}, "a@b.c"),
		},
	}
	stats := run(t, store, &fakeNotifier{err: context.DeadlineExceeded})

	if len(store.failed) != 2 {
		t.Errorf("failed = %v, want an attempt recorded against both members", store.failed)
	}
	if len(store.delivered) != 0 {
		t.Errorf("delivered = %v, want none", store.delivered)
	}
	if stats.Failed != 2 {
		t.Errorf("stats.Failed = %d, want 2", stats.Failed)
	}
}

// A multi-job group has no single slug to point at, so it records the job list the
// notification center's /jobs page renders — the shape a subscription digest uses.
func TestRun_MultiJobGroupRecordsOneListNotification(t *testing.T) {
	store := &fakeStore{
		due: []int64{1, 2},
		rows: map[int64]db.GetReminderForDeliveryRow{
			1: ownedBy(1, 42, []string{"email"}, "a@b.c"),
			2: ownedBy(2, 42, []string{"email"}, "a@b.c"),
		},
	}
	run(t, store, &fakeNotifier{})

	if len(store.recorded) != 1 {
		t.Fatalf("recorded = %v, want exactly one notification for the group", store.recorded)
	}
	rec := store.recorded[0]
	if rec.PublicSlug.Valid {
		t.Errorf("PublicSlug = %+v, want unset for a multi-job group", rec.PublicSlug)
	}
	// Unmarshalled into the SHARED shape, not a local copy of it: the column's
	// contract is notify.SnapshotJob, and a private struct here would keep passing
	// while the two drifted apart.
	var jobs []notify.SnapshotJob
	if err := json.Unmarshal(rec.Jobs, &jobs); err != nil {
		t.Fatalf("Jobs is not a job list: %v (raw %s)", err, rec.Jobs)
	}
	if len(jobs) != 2 || jobs[0].Slug != "job-1" || jobs[1].Slug != "job-2" {
		t.Errorf("Jobs = %+v, want both saved jobs in claim order", jobs)
	}
}

// A group of one is the message and the record we already shipped.
func TestRun_SingleJobGroupKeepsTheSlugRecord(t *testing.T) {
	store := &fakeStore{
		due:  []int64{1},
		rows: map[int64]db.GetReminderForDeliveryRow{1: ownedBy(1, 42, []string{"email"}, "a@b.c")},
	}
	run(t, store, &fakeNotifier{})

	rec := store.recorded[0]
	if !rec.PublicSlug.Valid || rec.PublicSlug.String != "job-1" {
		t.Errorf("PublicSlug = %+v, want the single job's slug", rec.PublicSlug)
	}
	if rec.Jobs != nil {
		t.Errorf("Jobs = %s, want unset for a single-job group", rec.Jobs)
	}
}

// job_reminders snapshots the channel set at schedule time (migration 0034), so two
// reminders of one account can carry different sets. Merging them would send one over
// the other's channels and stamp it delivered anyway.
func TestRun_DifferentChannelSetsAreNotMerged(t *testing.T) {
	both := ownedBy(2, 42, []string{"email", "telegram"}, "a@b.c")
	both.TelegramChatID = pgtype.Int8{Int64: 555, Valid: true}
	store := &fakeStore{
		due: []int64{1, 2},
		rows: map[int64]db.GetReminderForDeliveryRow{
			1: ownedBy(1, 42, []string{"email"}, "a@b.c"),
			2: both,
		},
	}
	notifier := &fakeNotifier{}
	run(t, store, notifier)

	// One message for the email-only reminder, two for the one that also wants
	// Telegram — three sends, and never a job in a message it did not belong to.
	if len(notifier.sent) != 3 {
		t.Fatalf("sent = %v, want the two channel sets kept apart", notifier.sent)
	}
	for i, group := range notifier.groups {
		if len(group) != 1 {
			t.Errorf("group %d carried %d jobs, want 1", i, len(group))
		}
	}
}

// The same account's reminders group even when their channel sets were stored in a
// different order — the key is the SET, not the slice.
func TestRun_ChannelOrderDoesNotSplitTheGroup(t *testing.T) {
	second := ownedBy(2, 42, []string{"telegram", "email"}, "a@b.c")
	second.TelegramChatID = pgtype.Int8{Int64: 555, Valid: true}
	first := ownedBy(1, 42, []string{"email", "telegram"}, "a@b.c")
	first.TelegramChatID = pgtype.Int8{Int64: 555, Valid: true}
	store := &fakeStore{
		due:  []int64{1, 2},
		rows: map[int64]db.GetReminderForDeliveryRow{1: first, 2: second},
	}
	notifier := &fakeNotifier{}
	run(t, store, notifier)

	// One batch, delivered over both of its channels.
	if len(notifier.groups) != 2 {
		t.Fatalf("sent = %v, want one batch over two channels", notifier.sent)
	}
	for i, group := range notifier.groups {
		if len(group) != 2 {
			t.Errorf("group %d carried %d jobs, want both", i, len(group))
		}
	}
}

// Beyond SnapshotCap the excess is RELEASED, never stamped: a reminder marked
// delivered while appearing in no message is gone for good.
func TestRun_BatchOverflowIsReleasedNotDelivered(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SnapshotCap = 2
	store := &fakeStore{
		due: []int64{1, 2, 3, 4},
		rows: map[int64]db.GetReminderForDeliveryRow{
			1: ownedBy(1, 42, []string{"email"}, "a@b.c"),
			2: ownedBy(2, 42, []string{"email"}, "a@b.c"),
			3: ownedBy(3, 42, []string{"email"}, "a@b.c"),
			4: ownedBy(4, 42, []string{"email"}, "a@b.c"),
		},
	}
	notifier := &fakeNotifier{}
	r := NewRunner(store, notifier, cfg)
	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(notifier.groups) != 1 || len(notifier.groups[0]) != 2 {
		t.Fatalf("groups = %v, want one message carrying the cap", notifier.groups)
	}
	if len(store.delivered) != 2 {
		t.Errorf("delivered = %v, want only what the message carried", store.delivered)
	}
	if len(store.released) != 2 || len(store.failed) != 0 {
		t.Errorf("released = %v failed = %v, want the overflow released and no attempt burnt", store.released, store.failed)
	}
	if stats.Deferred != 2 {
		t.Errorf("stats.Deferred = %d, want the 2 overflow reminders counted", stats.Deferred)
	}
}

// A hand-built Config with no SnapshotCap would make every batch full at zero
// members: nothing delivered, nothing failed, forever.
func TestNewRunner_ZeroSnapshotCapDoesNotStallDelivery(t *testing.T) {
	store := &fakeStore{
		due:  []int64{1},
		rows: map[int64]db.GetReminderForDeliveryRow{1: ownedBy(1, 42, []string{"email"}, "a@b.c")},
	}
	notifier := &fakeNotifier{}
	r := NewRunner(store, notifier, Config{LeaseSeconds: 600, ClaimBatch: 500, MaxAttempts: 5})
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.delivered) != 1 {
		t.Errorf("delivered = %v, want the reminder delivered despite an unset SnapshotCap", store.delivered)
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
