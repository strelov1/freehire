package mentorship

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWithdrawalCancelsFutureBookingsAndNotifiesEachSeeker(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	svc := New(repo, Config{
		Notifier: notifier,
		Now:      func() time.Time { return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC) },
	})
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	repo.futureBookings[submitted.ID] = []Booking{
		{ID: uuid.New(), SeekerUserID: 11},
		{ID: uuid.New(), SeekerUserID: 12},
	}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	if len(notifier.cancelled) != 2 {
		t.Errorf("notified %d seekers, want 2 — a session cancelled without telling anybody "+
			"is worse than one not cancelled", len(notifier.cancelled))
	}
	if !repo.withdrawn {
		t.Error("the profile was not withdrawn")
	}
	if repo.cancelledBeforeDelete != 2 {
		t.Errorf("%d bookings were cancelled before the delete, want 2 — the ON DELETE "+
			"CASCADE would otherwise take them and nobody would ever be told",
			repo.cancelledBeforeDelete)
	}
}

// A withdrawing mentor's cancelled bookings must not leave stray Meet events live on
// their own calendar — the same cleanup a single Cancel() already does, extended to the
// bulk path Withdraw uses.
func TestWithdrawalDeletesCalendarEventsOfCancelledBookings(t *testing.T) {
	repo := newFakeRepo()
	linker := &fakeCalendarLinker{}
	svc := New(repo, Config{
		Notifier:       &fakeNotifier{},
		CalendarLinker: linker,
		Now:            func() time.Time { return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC) },
	})
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	repo.futureBookings[submitted.ID] = []Booking{
		{ID: uuid.New(), SeekerUserID: 11, MentorUserID: submitted.UserID, GoogleEventID: "evt-1"},
		// No calendar event to clean up — the ordinary static-link case.
		{ID: uuid.New(), SeekerUserID: 12, MentorUserID: submitted.UserID},
	}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}
	if linker.deletedEventID != "evt-1" {
		t.Errorf("DeleteMeetEvent called with %q, want evt-1", linker.deletedEventID)
	}
}

// A delivery failure must not leave a mentor unable to withdraw. The cancellations have
// already committed; refusing here would strand them.
func TestWithdrawalSurvivesAFailedNotification(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{err: errors.New("smtp is down")}
	svc := New(repo, Config{Notifier: notifier, Now: time.Now})
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	repo.futureBookings[submitted.ID] = []Booking{{ID: uuid.New(), SeekerUserID: 11}}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Errorf("Withdraw: %v — a delivery failure must not block a withdrawal", err)
	}
	if !repo.withdrawn {
		t.Error("the profile was not withdrawn after a failed notification")
	}
}

func TestWithdrawalByAStrangerRemovesNothing(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	if err := svc.Withdraw(context.Background(), 4242); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("error = %v, want ErrProfileNotFound", err)
	}
	if repo.withdrawn {
		t.Error("a stranger withdrew somebody else's profile")
	}
}

// Withdrawal must not touch the mentor's own pause switch — status and pause are
// independent decisions, and folding them together is what made a withdrawn profile
// show up in the UI labelled "paused".
func TestWithdrawalDoesNotPauseTheProfile(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	profile, _, err := svc.MyProfile(context.Background(), validInput().UserID)
	if err != nil {
		t.Fatalf("MyProfile: %v", err)
	}
	if profile.Status != StatusWithdrawn {
		t.Errorf("status = %q, want %q", profile.Status, StatusWithdrawn)
	}
	if profile.Paused {
		t.Error("withdrawing paused the profile — status and pause must stay independent")
	}
}

// A second withdrawal changes nothing, per the spec — it must succeed, not 404 as
// though the profile had vanished.
func TestASecondWithdrawalSucceeds(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("first Withdraw: %v", err)
	}
	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Errorf("second Withdraw: %v, want success — a repeat withdrawal is a no-op, not a failure", err)
	}
}

// Reactivating a withdrawn profile sends it back into the ordinary moderation queue —
// exactly the pending state a first submission starts in, with no special treatment for
// having been a mentor before.
func TestReactivateReturnsAWithdrawnProfileToPending(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	submitted, err := svc.SubmitProfile(context.Background(), validInput())
	if err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if _, err := svc.Decide(context.Background(), submitted.ID, 99, StatusApproved); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	reactivated, err := svc.Reactivate(context.Background(), validInput().UserID)
	if err != nil {
		t.Fatalf("Reactivate: %v", err)
	}
	if reactivated.Status != StatusPending {
		t.Errorf("status = %q, want %q", reactivated.Status, StatusPending)
	}
	if reactivated.Paused {
		t.Error("a reactivated profile is paused")
	}
}

// Resubmitting is not a way to force a pending or approved profile back to review out
// of turn — only a withdrawn one may be reactivated.
func TestReactivateRefusesAProfileThatWasNeverWithdrawn(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}

	if _, err := svc.Reactivate(context.Background(), validInput().UserID); !errors.Is(err, ErrProfileNotWithdrawn) {
		t.Errorf("error = %v, want ErrProfileNotWithdrawn", err)
	}
}

func TestReactivateByAStrangerIsRefused(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(t, repo)
	if _, err := svc.SubmitProfile(context.Background(), validInput()); err != nil {
		t.Fatalf("SubmitProfile: %v", err)
	}
	if err := svc.Withdraw(context.Background(), validInput().UserID); err != nil {
		t.Fatalf("Withdraw: %v", err)
	}

	if _, err := svc.Reactivate(context.Background(), 4242); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("error = %v, want ErrProfileNotFound", err)
	}
}
