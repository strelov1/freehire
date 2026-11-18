package nudge

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestDeliver_SoftSkipsWhenNoDestination(t *testing.T) {
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindFollowUp, "applied", 25, true, true, []string{"telegram"}, nil, "")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("must not send with no destination, sent %v", notifier.sent)
	}
	if len(store.released) != 1 {
		t.Errorf("must release the claim, released = %v", store.released)
	}
	if stats.SoftSkips != 1 {
		t.Errorf("stats.SoftSkips = %d, want 1", stats.SoftSkips)
	}
}

func TestDeliver_RecordsFailureOnDeliveryError(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindFollowUp, "applied", 25, true, true, []string{"telegram"}, &chat, "")}
	notifier := &fakeNotifier{err: errors.New("telegram down")}
	r := newRunner(store, notifier)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.failed) != 1 {
		t.Errorf("must record failure, failed = %v", store.failed)
	}
	if len(store.delivered) != 0 {
		t.Errorf("must not mark delivered on error, delivered = %v", store.delivered)
	}
	if stats.Failed != 1 {
		t.Errorf("stats.Failed = %d, want 1", stats.Failed)
	}
}

func TestDeliver_EmailDestinationIsAccountEmail(t *testing.T) {
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindFollowUp, "applied", 25, true, true, []string{"email"}, nil, "user@acme.com")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || notifier.sent[0] != "email:user@acme.com" {
		t.Errorf("sent = %v, want [email:user@acme.com]", notifier.sent)
	}
}

// --- DELIVER: notification-center recording -----------------------------------

func TestDeliver_RecordsNotificationForEachKind(t *testing.T) {
	cases := []struct {
		kind      string
		wantKind  string
		wantTitle string
		wantBody  string
	}{
		{KindFollowUp, "nudge_follow_up", "👋 Follow up?", "It's been 25 days since anything moved on Go Dev at Acme."},
		{KindInterviewPrep, "nudge_interview_prep", "🎯 Interview coming up", "You're interviewing for Go Dev at Acme."},
		{KindJobClosed, "nudge_job_closed", "📪 Job closed", "Go Dev at Acme was closed."},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			chat := int64(555)
			stage := "applied"
			jobOpen := true
			switch tc.kind {
			case KindInterviewPrep:
				stage = "interview"
			case KindJobClosed:
				stage = "screening"
				jobOpen = false
			}
			row := deliveryRow(tc.kind, stage, 25, true, jobOpen, []string{"telegram"}, &chat, "")
			row.UserID = 99
			store := &fakeStore{due: []int64{1}, row: row}
			notifier := &fakeNotifier{}
			r := newRunner(store, notifier)

			stats, err := r.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(store.recordedNotifications) != 1 {
				t.Fatalf("recordedNotifications = %d, want 1", len(store.recordedNotifications))
			}
			got := store.recordedNotifications[0]
			if got.UserID != 99 {
				t.Errorf("UserID = %d, want 99", got.UserID)
			}
			if got.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if got.Title != tc.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tc.wantTitle)
			}
			if got.Body != tc.wantBody {
				t.Errorf("Body = %q, want %q", got.Body, tc.wantBody)
			}
			if !got.PublicSlug.Valid || got.PublicSlug.String != "go-dev-acme" {
				t.Errorf("PublicSlug = %+v, want valid go-dev-acme", got.PublicSlug)
			}
			if stats.Delivered != 1 {
				t.Errorf("stats.Delivered = %d, want 1", stats.Delivered)
			}
		})
	}
}

func TestDeliver_RecordNotificationFailureDoesNotBlockDelivery(t *testing.T) {
	chat := int64(555)
	row := deliveryRow(KindFollowUp, "applied", 25, true, true, []string{"telegram"}, &chat, "")
	row.UserID = 99
	store := &fakeStore{due: []int64{1}, row: row, notifyErr: errors.New("insert failed")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.delivered) != 1 {
		t.Errorf("delivered = %v, want [1] (a notification-record failure must not block delivery)", store.delivered)
	}
	if stats.Delivered != 1 {
		t.Errorf("stats.Delivered = %d, want 1", stats.Delivered)
	}
}

// --- recipient: push ----------------------------------------------------------

func TestRecipient_Push_SoftSkipsWithNoRegisteredDevice(t *testing.T) {
	info := db.GetNudgeForDeliveryRow{UserID: 42, HasPushDevice: false}

	dest, ok := recipient(notify.ChannelPush, info)
	if ok {
		t.Fatal("ok = true, want false (no registered device)")
	}
	if dest != "" {
		t.Errorf("dest = %q, want empty", dest)
	}
}

// --- DELIVER: quiet hours ------------------------------------------------

func TestDeliver_DeferredDuringQuietHours(t *testing.T) {
	// fixedNow is 12:00 UTC; an 11:00-13:00 window covers it.
	chat := int64(555)
	row := deliveryRow(KindFollowUp, "applied", 25, true, true, []string{"telegram"}, &chat, "")
	row.Timezone = pgtype.Text{String: "UTC", Valid: true}
	row.QuietHoursStart = pgTime(11, 0)
	row.QuietHoursEnd = pgTime(13, 0)
	store := &fakeStore{due: []int64{1}, row: row}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

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

func TestDeliver_DeliversOutsideQuietHours(t *testing.T) {
	// fixedNow is 12:00 UTC; a 22:00-08:00 window does not cover it.
	chat := int64(555)
	row := deliveryRow(KindFollowUp, "applied", 25, true, true, []string{"telegram"}, &chat, "")
	row.Timezone = pgtype.Text{String: "UTC", Valid: true}
	row.QuietHoursStart = pgTime(22, 0)
	row.QuietHoursEnd = pgTime(8, 0)
	store := &fakeStore{due: []int64{1}, row: row}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

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

// Same failure as the digest and reminder workers': a user who blocked the bot
// answered every telegram send with 403, and each nudge counted that as a delivery
// failure to retry. Retrying cannot reach a closed chat, so the link goes instead.
func TestDeliver_RecipientGoneUnlinksTelegramInsteadOfFailing(t *testing.T) {
	chat := int64(555)
	row := deliveryRow(KindFollowUp, "applied", 25, true, true, []string{"telegram"}, &chat, "")
	row.UserID = 42
	store := &fakeStore{due: []int64{1}, row: row}
	r := newRunner(store, &fakeNotifier{err: ErrRecipientGone})

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.unlinkedTelegram) != 1 || store.unlinkedTelegram[0] != 42 {
		t.Errorf("unlinkedTelegram = %v, want [42]", store.unlinkedTelegram)
	}
	if stats.Failed != 0 {
		t.Errorf("stats.Failed = %d, want 0 — a chat that will never accept us is not a failure to retry", stats.Failed)
	}
	if len(store.failed) != 0 {
		t.Errorf("failed = %v, want none", store.failed)
	}
}

// A nudge is delivered if ANY of its channels lands. A dead Telegram chat must not
// cancel the email that did arrive.
func TestDeliver_RecipientGoneOnOneChannelStillDeliversTheOther(t *testing.T) {
	chat := int64(555)
	row := deliveryRow(KindFollowUp, "applied", 25, true, true, []string{"telegram", "email"}, &chat, "a@b.c")
	row.UserID = 42
	store := &fakeStore{due: []int64{1}, row: row}
	notifier := &fakeNotifier{errByChannel: map[string]error{"telegram": ErrRecipientGone}}
	r := newRunner(store, notifier)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.delivered) != 1 {
		t.Errorf("delivered = %v, want the nudge delivered over email", store.delivered)
	}
	if stats.Failed != 0 {
		t.Errorf("stats.Failed = %d, want 0", stats.Failed)
	}
	if len(store.unlinkedTelegram) != 1 {
		t.Errorf("unlinkedTelegram = %v, want the dead chat forgotten anyway", store.unlinkedTelegram)
	}
}
