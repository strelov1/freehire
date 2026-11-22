package nudge

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestDeliver_FollowUp_DeliversWhenStillSilent(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindFollowUp, "applied", 25, true, true, []string{"telegram"}, &chat, "")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || notifier.sent[0] != "telegram:555" {
		t.Errorf("sent = %v, want [telegram:555]", notifier.sent)
	}
	if len(store.delivered) != 1 {
		t.Errorf("delivered = %v, want [1]", store.delivered)
	}
	if stats.Delivered != 1 {
		t.Errorf("stats.Delivered = %d, want 1", stats.Delivered)
	}
}

func TestDeliver_FollowUp_CancelsWhenNoLongerSilent(t *testing.T) {
	// 5 days silent is back within "applied"'s tolerance by delivery time — a
	// reply must have arrived between MATCH and DELIVER.
	chat := int64(555)
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindFollowUp, "applied", 5, true, true, []string{"telegram"}, &chat, "")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("must not send a stale follow-up, sent %v", notifier.sent)
	}
	if len(store.cancelled) != 1 {
		t.Errorf("must cancel at fire, cancelled = %v", store.cancelled)
	}
	if stats.Cancelled != 1 {
		t.Errorf("stats.Cancelled = %d, want 1", stats.Cancelled)
	}
}

func TestDeliver_InterviewPrep_DeliversWhileStillInterview(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindInterviewPrep, "interview", 0, true, true, []string{"telegram"}, &chat, "")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Errorf("sent = %v, want 1 message", notifier.sent)
	}
	if len(store.delivered) != 1 {
		t.Errorf("delivered = %v, want [1]", store.delivered)
	}
}

func TestDeliver_InterviewPrep_CancelsWhenStageMoved(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindInterviewPrep, "offer", 0, true, true, []string{"telegram"}, &chat, "")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("must not send once the stage has moved on, sent %v", notifier.sent)
	}
	if len(store.cancelled) != 1 {
		t.Errorf("must cancel at fire, cancelled = %v", store.cancelled)
	}
}

func TestDeliver_JobClosed_DeliversWhileApplicationStillActive(t *testing.T) {
	chat := int64(555)
	// jobOpen=false: the job stays closed, which is the whole point of this kind.
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindJobClosed, "screening", 0, true, false, []string{"telegram"}, &chat, "")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Errorf("sent = %v, want 1 message", notifier.sent)
	}
	if len(store.delivered) != 1 {
		t.Errorf("delivered = %v, want [1]", store.delivered)
	}
}

func TestDeliver_JobClosed_CancelsWhenApplicationSettled(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindJobClosed, "withdrawn", 0, true, false, []string{"telegram"}, &chat, "")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("must not send once the application has settled, sent %v", notifier.sent)
	}
	if len(store.cancelled) != 1 {
		t.Errorf("must cancel at fire, cancelled = %v", store.cancelled)
	}
}

func TestDeliver_CancelsWhenNotificationsDisabled(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindFollowUp, "applied", 25, false, true, []string{"telegram"}, &chat, "")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("must not send once notifications are off, sent %v", notifier.sent)
	}
	if len(store.cancelled) != 1 {
		t.Errorf("must cancel at fire, cancelled = %v", store.cancelled)
	}
}

func TestDeliver_CancelsWhenJobClosed(t *testing.T) {
	chat := int64(555)
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindFollowUp, "applied", 25, true, false, []string{"telegram"}, &chat, "")}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.cancelled) != 1 {
		t.Errorf("closed job must be cancelled, cancelled = %v", store.cancelled)
	}
}

// untrackedRow simulates a nudge whose application row was deleted (by
// UntrackJob/UntrackApplicationByID) between MATCH and DELIVER: the LEFT JOIN
// leaves Stage invalid, same as a row that exists with a NULL stage, but
// ApplicationExists is false only for the former.
func untrackedRow(kind string) db.GetNudgeForDeliveryRow {
	chat := int64(555)
	row := deliveryRow(kind, "", 25, true, true, []string{"telegram"}, &chat, "")
	row.ApplicationExists = false
	return row
}

func TestActionable_FailsClosedWhenApplicationGone(t *testing.T) {
	for _, kind := range []string{KindFollowUp, KindInterviewPrep, KindJobClosed} {
		t.Run(kind, func(t *testing.T) {
			r := newRunner(&fakeStore{}, &fakeNotifier{})
			row := untrackedRow(kind)
			if kind == KindInterviewPrep {
				// Satisfy the pre-existing stage=="interview" check too, so this
				// case actually exercises the ApplicationExists gate rather than
				// passing on the stage check alone regardless of it.
				row.Stage = pgtype.Text{String: "interview", Valid: true}
			}
			if r.actionable(row) {
				t.Errorf("actionable() = true for an untracked application, want false (fail closed)")
			}
		})
	}
}

// The three auto-apply outcome kinds are the deliberate opposite of
// TestActionable_FailsClosedWhenApplicationGone above: their condition cannot lapse,
// so actionable() ignores stage/job-open/application-existence entirely and answers
// only off the NotificationsEnabled gate.
func TestActionable_AutoApplyOutcomeKinds_IgnoreEverythingButNotificationsEnabled(t *testing.T) {
	for _, kind := range []string{KindAutoApplySubmitted, KindAutoApplyBlocked, KindAutoApplyFailed} {
		t.Run(kind, func(t *testing.T) {
			r := newRunner(&fakeStore{}, &fakeNotifier{})
			row := untrackedRow(kind) // no tracked application, job closed — still actionable
			if !r.actionable(row) {
				t.Errorf("actionable() = false for %s with notifications enabled, want true", kind)
			}
			row.NotificationsEnabled = false
			if r.actionable(row) {
				t.Errorf("actionable() = true for %s with notifications disabled, want false", kind)
			}
		})
	}
}

func TestDeliver_CancelsWhenApplicationGone(t *testing.T) {
	// Stage.Valid=false with ApplicationExists=false must not be read as the
	// active `applied` stage, the way an invalid-but-still-tracked stage is.
	store := &fakeStore{due: []int64{1}, row: untrackedRow(KindFollowUp)}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Errorf("must not send once the application is untracked, sent %v", notifier.sent)
	}
	if len(store.cancelled) != 1 {
		t.Errorf("must cancel at fire, cancelled = %v", store.cancelled)
	}
}

// A review-ready reminder is worth sending only while the job is open and the entry still waits.
func TestActionable_AutoApplyReviewReady_OnlyWhileJobOpen(t *testing.T) {
	r := newRunner(&fakeStore{}, &fakeNotifier{})
	row := untrackedRow(KindAutoApplyReviewReady)
	row.JobOpen, row.AutoApplyReviewPending = true, true
	if !r.actionable(row) {
		t.Error("actionable() = false for an open job still in review, want true")
	}
	row.JobOpen = false
	if r.actionable(row) {
		t.Error("actionable() = true for a closed job, want false")
	}
	// Decided (approved, declined, parked) while the nudge waited for delivery.
	row.JobOpen, row.AutoApplyReviewPending = true, false
	if r.actionable(row) {
		t.Error("actionable() = true for an entry no longer in review, want false")
	}
}
