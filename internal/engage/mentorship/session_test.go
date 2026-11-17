package mentorship

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// A booking is readable only by its two parties. The random identifier makes bookings
// unenumerable; this makes a leaked or guessed one useless. Neither substitutes for the
// other, which is why the spec states them as separate requirements.
func TestOnlyThePartiesToASessionMayReadIt(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	svc := bookingService(repo, notifier)

	const seeker = 42
	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: seeker,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	t.Run("the seeker may", func(t *testing.T) {
		got, err := svc.Session(context.Background(), booking.ID, seeker)
		if err != nil {
			t.Fatalf("Session: %v", err)
		}
		if got.ID != booking.ID {
			t.Error("a different booking came back")
		}
		if got.MeetingURL == "" {
			t.Error("the seeker's own booking carries no meeting link")
		}
	})

	t.Run("the mentor may", func(t *testing.T) {
		if _, err := svc.Session(context.Background(), booking.ID, mentor.UserID); err != nil {
			t.Errorf("Session: %v", err)
		}
	})

	// The same answer a booking that does not exist gets — a stranger must not learn that
	// somebody else's session is real.
	t.Run("a stranger may not", func(t *testing.T) {
		if _, err := svc.Session(context.Background(), booking.ID, 4242); !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("error = %v, want ErrBookingNotFound", err)
		}
	})

	t.Run("an unknown id answers the same way", func(t *testing.T) {
		if _, err := svc.Session(context.Background(), uuid.New(), seeker); !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("error = %v, want ErrBookingNotFound", err)
		}
	})

	t.Run("an unauthenticated caller is refused", func(t *testing.T) {
		if _, err := svc.Session(context.Background(), booking.ID, 0); !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("error = %v, want ErrNotAuthenticated", err)
		}
	})
}

// Completion is not a decision anybody makes; it is "confirmed, and the end has passed".
func TestASessionIsCompleteOnceItsEndHasPassed(t *testing.T) {
	start := time.Date(2026, time.September, 8, 18, 0, 0, 0, time.UTC)
	booking := Booking{StartsAt: start, EndsAt: start.Add(time.Hour), Status: BookingConfirmed}

	for _, tc := range []struct {
		name string
		now  time.Time
		want bool
	}{
		{"before it starts", start.Add(-time.Hour), false},
		{"while it runs", start.Add(30 * time.Minute), false},
		{"at the moment it ends", start.Add(time.Hour), true},
		{"afterwards", start.Add(2 * time.Hour), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := booking.Completed(tc.now); got != tc.want {
				t.Errorf("Completed = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("a cancelled session is never complete", func(t *testing.T) {
		cancelled := booking
		cancelled.Status = BookingCancelled
		if cancelled.Completed(start.Add(2 * time.Hour)) {
			t.Error("a cancelled session reports itself as completed")
		}
	})
}

func TestOnlyTheSeekerOfACompletedSessionMayReviewIt(t *testing.T) {
	repo := newFakeRepo()
	mentor := bookableMentor(t, repo)
	svc := New(repo, Config{Now: func() time.Time {
		// A week after the session.
		return time.Date(2026, time.September, 15, 9, 0, 0, 0, time.UTC)
	}})

	past := Booking{
		ID: uuid.New(), MentorID: mentor.ID, MentorUserID: mentor.UserID, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), EndsAt: tuesdayAt(t, 19), Status: BookingConfirmed,
	}
	repo.bookings[past.ID] = past

	t.Run("the seeker may", func(t *testing.T) {
		if _, err := svc.Review(context.Background(), past.ID, 42, 5, "great"); err != nil {
			t.Errorf("Review: %v", err)
		}
	})

	t.Run("the mentor may not", func(t *testing.T) {
		if _, err := svc.Review(context.Background(), past.ID, mentor.UserID, 5, ""); !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("error = %v, want ErrBookingNotFound", err)
		}
	})

	t.Run("a stranger may not", func(t *testing.T) {
		if _, err := svc.Review(context.Background(), past.ID, 4242, 5, ""); !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("error = %v, want ErrBookingNotFound", err)
		}
	})

	t.Run("a rating outside one to five is refused", func(t *testing.T) {
		for _, rating := range []int{0, -1, 6, 100} {
			if _, err := svc.Review(context.Background(), past.ID, 42, rating, ""); !errors.Is(err, ErrInvalidReview) {
				t.Errorf("rating %d: error = %v, want ErrInvalidReview", rating, err)
			}
		}
	})
}

func TestASessionThatHasNotHappenedCannotBeReviewed(t *testing.T) {
	repo := newFakeRepo()
	mentor := bookableMentor(t, repo)
	svc := bookingService(repo, nil)

	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	if _, err := svc.Review(context.Background(), booking.ID, 42, 5, ""); !errors.Is(err, ErrSessionNotCompleted) {
		t.Errorf("error = %v, want ErrSessionNotCompleted", err)
	}

	t.Run("nor can a cancelled one", func(t *testing.T) {
		if _, err := svc.Cancel(context.Background(), booking.ID, 42, ""); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		late := New(repo, Config{Now: func() time.Time {
			return time.Date(2026, time.September, 15, 9, 0, 0, 0, time.UTC)
		}})
		if _, err := late.Review(context.Background(), booking.ID, 42, 5, ""); !errors.Is(err, ErrSessionNotCompleted) {
			t.Errorf("error = %v, want ErrSessionNotCompleted", err)
		}
	})
}
