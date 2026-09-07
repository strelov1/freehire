package mentorship

import (
	"context"
	"errors"
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
	repo.bookings[uuid.New()] = remindableBooking(t, 3*time.Hour)    // inside 24h, outside 1h
	repo.bookings[uuid.New()] = remindableBooking(t, 30*time.Minute) // inside both

	stats, err := reminderService(repo, notifier).SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}

	// Three reminders: the far session's 24h one, and both of the near session's.
	if stats.Sent != 3 {
		t.Errorf("sent = %d, want 3 (offsets fired: %v)", stats.Sent, notifier.reminders)
	}
}

// The claim is what makes this idempotent, and it must happen BEFORE the send: a worker
// that sends first and records afterwards sends twice whenever the second step fails.
func TestARerunSendsNothingTwice(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	repo.bookings[uuid.New()] = remindableBooking(t, 3*time.Hour)
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
	repo.bookings[uuid.New()] = remindableBooking(t, 3*time.Hour)
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
	booking := remindableBooking(t, 3*time.Hour)
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
		repo.bookings[uuid.New()] = remindableBooking(t, 3*time.Hour)
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
	repo.bookings[uuid.New()] = remindableBooking(t, 3*time.Hour)

	stats, err := New(repo, Config{Now: func() time.Time { return reminderNow }}).
		SendDueReminders(context.Background(), 100)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}
	if stats.Sent != 0 || repo.claimed != 0 {
		t.Errorf("claimed %d reminders with no way to send them", repo.claimed)
	}
}
