package mentorship

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

// remindableBooking is a confirmed session at a known distance from now.
func remindableBooking(t *testing.T, in time.Duration) Booking {
	t.Helper()
	start := reminderNow.Add(in)
	return Booking{
		ID:             uuid.New(),
		MentorID:       1,
		MentorEmail:    "mentor@example.test",
		MentorTimezone: "Europe/Berlin",
		SeekerUserID:   42,
		SeekerEmail:    "seeker@example.test",
		SeekerTimezone: "Asia/Tokyo",
		StartsAt:       start,
		EndsAt:         start.Add(time.Hour),
		Status:         BookingConfirmed,
	}
}

var reminderNow = time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC)

func reminderService(repo *fakeRepo, notifier Notifier) *Service {
	return New(repo, Config{Notifier: notifier, Now: func() time.Time { return reminderNow }})
}

func TestRemindersFireAtBothOffsets(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	repo.bookings[uuid.New()] = remindableBooking(t, 23*time.Hour+30*time.Minute) // the 24h window
	repo.bookings[uuid.New()] = remindableBooking(t, 30*time.Minute)              // the 1h window

	stats, err := reminderService(repo, notifier).SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}

	if stats.Sent != 2 {
		t.Errorf("sent = %d, want 2 (offsets fired: %v)", stats.Sent, notifier.reminders)
	}
	if !slices.Contains(notifier.reminders, 24*time.Hour) || !slices.Contains(notifier.reminders, time.Hour) {
		t.Errorf("offsets fired: %v, want one of each", notifier.reminders)
	}
}

// The bug the lower bound exists to prevent: a session booked three hours out has no
// 24-hour reminder to give, and sending one anyway would say "in 24 hours" about a
// meeting three hours away. It still gets its 1-hour reminder, later.
//
// The previous version of this file ASSERTED the old behaviour — it booked three hours
// out and required the 24-hour reminder to fire. A test can hold a bug in place as firmly
// as it holds a feature.
func TestASessionBookedInsideAnOffsetGetsNoReminderForThatOffset(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	repo.bookings[uuid.New()] = remindableBooking(t, 3*time.Hour)

	stats, err := reminderService(repo, notifier).SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}

	if stats.Sent != 0 {
		t.Errorf("sent %d reminders for a session three hours out: %v", stats.Sent, notifier.reminders)
	}
}

// Each offset's window is bounded on both sides, so a session passes through exactly one
// of them per offset rather than every wider one at once.
func TestEachOffsetFiresInItsOwnWindowOnly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		until time.Duration
		want  []time.Duration
	}{
		{"a day and a half out", 36 * time.Hour, nil},
		{"just inside the 24h window", 23*time.Hour + 55*time.Minute, []time.Duration{24 * time.Hour}},
		{"just outside it", 22*time.Hour + 55*time.Minute, nil},
		{"in the quiet middle", 6 * time.Hour, nil},
		{"just inside the 1h window", 55 * time.Minute, []time.Duration{time.Hour}},
		{"minutes away", 5 * time.Minute, []time.Duration{time.Hour}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			notifier := &fakeNotifier{}
			repo.bookings[uuid.New()] = remindableBooking(t, tc.until)

			if _, err := reminderService(repo, notifier).SendDueReminders(context.Background(), 100); err != nil {
				t.Fatalf("SendDueReminders: %v", err)
			}
			if !slices.Equal(notifier.reminders, tc.want) {
				t.Errorf("fired %v, want %v", notifier.reminders, tc.want)
			}
		})
	}
}

// The claim is what makes this idempotent, and it must happen BEFORE the send: a worker
// that sends first and records afterwards sends twice whenever the second step fails.
func TestARerunSendsNothingTwice(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	repo.bookings[uuid.New()] = remindableBooking(t, 30*time.Minute)
	svc := reminderService(repo, notifier)

	first, err := svc.SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := svc.SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if first.Sent != 1 {
		t.Errorf("first run sent %d, want 1", first.Sent)
	}
	if second.Sent != 0 {
		t.Errorf("second run sent %d, want 0", second.Sent)
	}
}

// A claim lost to a concurrent run means somebody else is sending it. Sending anyway is
// the one outcome worse than not sending at all.
func TestALostClaimSendsNothing(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	repo.bookings[uuid.New()] = remindableBooking(t, 30*time.Minute)
	repo.claimAlwaysLost = true

	stats, err := reminderService(repo, notifier).SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}
	if stats.Sent != 0 || len(notifier.reminders) != 0 {
		t.Errorf("sent %d reminders after losing every claim, want 0", len(notifier.reminders))
	}
}

// A session that has already started gets nothing. "Your session starts in an hour",
// delivered afterwards, is worse than silence.
func TestASessionAlreadyStartedIsNotRemindedAbout(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	repo.bookings[uuid.New()] = remindableBooking(t, -30*time.Minute)

	stats, err := reminderService(repo, notifier).SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}
	if stats.Sent != 0 {
		t.Errorf("sent %d reminders for a session in the past, want 0", stats.Sent)
	}
}

func TestACancelledSessionIsNotRemindedAbout(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	booking := remindableBooking(t, 30*time.Minute)
	booking.Status = BookingCancelled
	repo.bookings[booking.ID] = booking

	stats, err := reminderService(repo, notifier).SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}
	if stats.Sent != 0 {
		t.Errorf("sent %d reminders for a cancelled session, want 0", stats.Sent)
	}
}

// One failed delivery must not stop the run: the other sessions still need theirs.
func TestOneFailedReminderDoesNotStopTheRun(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{err: errors.New("smtp is down")}
	for range 3 {
		repo.bookings[uuid.New()] = remindableBooking(t, 30*time.Minute)
	}

	stats, err := reminderService(repo, notifier).SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("SendDueReminders: %v — a delivery failure is not a run failure", err)
	}
	if stats.Failed != 3 {
		t.Errorf("failed = %d, want 3", stats.Failed)
	}
	if stats.Sent != 0 {
		t.Errorf("sent = %d, want 0", stats.Sent)
	}
}

// A failed delivery gives its claim back, so the next run tries again. Holding it would
// mean a mail server down for one run loses that reminder permanently — and the two
// failures are not equal: a missing reminder costs somebody the session, a duplicate
// costs them a duplicate.
func TestAFailedDeliveryIsRetriedOnTheNextRun(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{err: errors.New("smtp is down")}
	booking := remindableBooking(t, 30*time.Minute)
	repo.bookings[booking.ID] = booking
	svc := reminderService(repo, notifier)

	first, err := svc.SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Failed != 1 || first.Sent != 0 {
		t.Fatalf("first run sent=%d failed=%d, want 0 and 1", first.Sent, first.Failed)
	}

	// The mail server comes back.
	notifier.err = nil
	second, err := svc.SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Sent != 1 {
		t.Errorf("second run sent %d, want 1 — the failed reminder was not retried", second.Sent)
	}

	// And having now succeeded, it is not sent a third time.
	third, err := svc.SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("third run: %v", err)
	}
	if third.Sent != 0 {
		t.Errorf("third run sent %d, want 0", third.Sent)
	}
}

// A release that itself fails leaves the reminder claimed and unsent. The run must not
// fail over it — the other sessions still need theirs — but it is the outcome the release
// exists to avoid, so it is logged rather than swallowed.
func TestAFailedReleaseDoesNotFailTheRun(t *testing.T) {
	repo := newFakeRepo()
	repo.releaseErr = errors.New("connection reset")
	notifier := &fakeNotifier{err: errors.New("smtp is down")}
	repo.bookings[uuid.New()] = remindableBooking(t, 30*time.Minute)

	stats, err := reminderService(repo, notifier).SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Errorf("SendDueReminders: %v — a failed release is not a run failure", err)
	}
	if stats.Failed != 1 {
		t.Errorf("failed = %d, want 1", stats.Failed)
	}
}

// Failing to READ the work is different from failing to deliver it: nothing was attempted,
// and the run must say so rather than reporting a quiet success.
func TestAFailureToReadTheWorkFailsTheRun(t *testing.T) {
	repo := newFakeRepo()
	repo.listDueErr = errors.New("connection refused")

	if _, err := reminderService(repo, &fakeNotifier{}).SendDueReminders(context.Background(), 100); err == nil {
		t.Error("a failed read reported success")
	}
}

// Without a notifier there is nothing to send, and claiming a reminder would mark it sent
// forever. The run must touch nothing.
func TestWithoutANotifierNothingIsClaimed(t *testing.T) {
	repo := newFakeRepo()
	repo.bookings[uuid.New()] = remindableBooking(t, 30*time.Minute)

	stats, err := New(repo, Config{Now: func() time.Time { return reminderNow }}).
		SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}
	if stats.Sent != 0 || repo.claimed != 0 {
		t.Errorf("claimed %d reminders with no way to send them", repo.claimed)
	}
}
