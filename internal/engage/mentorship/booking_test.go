package mentorship

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/application/gmailsync"
)

// bookableMentor is an approved mentor free every Tuesday 18:00–22:00 Berlin, hour-long
// sessions, no notice period unless a test sets one.
func bookableMentor(t *testing.T, repo *fakeRepo) Profile {
	t.Helper()
	in := validInput()
	in.Session.MinimumNotice = 0
	profile, err := repo.CreateProfile(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	profile, err = repo.DecideProfile(context.Background(), profile.ID, 99, StatusApproved)
	if err != nil {
		t.Fatalf("DecideProfile: %v", err)
	}

	rule, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 22, 0))
	if err != nil {
		t.Fatalf("NewWeeklyRule: %v", err)
	}
	repo.availability[profile.ID] = []Rule{rule}
	return profile
}

// tuesdayAt is an instant on Tuesday 8 September 2026, in the mentor's zone.
func tuesdayAt(t *testing.T, hour int) time.Time {
	t.Helper()
	zone := berlin(t)
	return time.Date(2026, time.September, 8, hour, 0, 0, 0, zone)
}

// bookingService is wired with a clock on the Monday before, so the whole Tuesday is
// ahead of "now".
func bookingService(repo *fakeRepo, notifier Notifier) *Service {
	return New(repo, Config{
		Notifier: notifier,
		Now: func() time.Time {
			return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC)
		},
	})
}

// bookingServiceWithCalendar is bookingService plus a CalendarLinker, for the tests that
// exercise the connected-mentor path.
func bookingServiceWithCalendar(repo *fakeRepo, notifier Notifier, calendar CalendarLinker) *Service {
	return New(repo, Config{
		Notifier:       notifier,
		CalendarLinker: calendar,
		Now: func() time.Time {
			return time.Date(2026, time.September, 7, 9, 0, 0, 0, time.UTC)
		},
	})
}

func TestBookingASlotConfirmsIt(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	svc := bookingService(repo, notifier)

	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug:     mentor.Slug,
		SeekerUserID:   42,
		StartsAt:       tuesdayAt(t, 18),
		SeekerTimezone: "Asia/Tokyo",
		Note:           "I want to talk about the backend team",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	if booking.Status != BookingConfirmed {
		t.Errorf("status = %q, want confirmed", booking.Status)
	}
	if !booking.EndsAt.Equal(booking.StartsAt.Add(mentor.Session.Duration)) {
		t.Errorf("session runs %v, want %v", booking.EndsAt.Sub(booking.StartsAt), mentor.Session.Duration)
	}
	// A snapshot, so a mentor changing their link later cannot rewrite an invitation
	// somebody already holds.
	if booking.MeetingURL != mentor.MeetingURL {
		t.Errorf("meeting link = %q, want the mentor's at booking time", booking.MeetingURL)
	}
	if len(notifier.confirmed) != 1 {
		t.Errorf("%d confirmations sent, want 1", len(notifier.confirmed))
	}
}

// The rule that makes a stale page safe: the slot is re-derived at write time, and a
// start that is no longer offerable is refused with a reason rather than written.
func TestBookingRefusesAStartThatIsNotAnOfferableSlot(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start func(t *testing.T) time.Time
	}{
		{"outside the mentor's hours", func(t *testing.T) time.Time { return tuesdayAt(t, 9) }},
		{"off the grid by half an hour", func(t *testing.T) time.Time { return tuesdayAt(t, 18).Add(30 * time.Minute) }},
		{"on a day the mentor is not available", func(t *testing.T) time.Time { return tuesdayAt(t, 18).AddDate(0, 0, 1) }},
		{"in the past", func(t *testing.T) time.Time { return tuesdayAt(t, 18).AddDate(0, 0, -7) }},
		{"beyond the booking horizon", func(t *testing.T) time.Time { return tuesdayAt(t, 18).AddDate(1, 0, 0) }},
		{"in the last slot's tail", func(t *testing.T) time.Time { return tuesdayAt(t, 22) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			mentor := bookableMentor(t, repo)

			_, err := bookingService(repo, nil).Book(context.Background(), BookingInput{
				MentorSlug: mentor.Slug, SeekerUserID: 42,
				StartsAt: tc.start(t), SeekerTimezone: "Asia/Tokyo",
			})
			if !errors.Is(err, ErrSlotUnavailable) {
				t.Errorf("error = %v, want ErrSlotUnavailable", err)
			}
		})
	}
}

// A slot inside the notice period was offerable an hour ago and is not now. It must be
// refused for the same reason a taken one is, so a seeker on a stale page sees one
// answer rather than two.
func TestBookingRefusesASlotInsideTheNoticePeriod(t *testing.T) {
	repo := newFakeRepo()
	mentor := bookableMentor(t, repo)
	profile := repo.profiles[mentor.ID]
	profile.Session.MinimumNotice = 48 * time.Hour
	repo.profiles[mentor.ID] = profile

	_, err := bookingService(repo, nil).Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Errorf("error = %v, want ErrSlotUnavailable", err)
	}
}

// A booked hour is not offerable, and the second seeker's refusal must be the ordinary
// one — not a constraint violation leaking out as a 500.
func TestBookingRefusesAnHourAlreadyTaken(t *testing.T) {
	repo := newFakeRepo()
	mentor := bookableMentor(t, repo)
	svc := bookingService(repo, nil)

	if _, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	}); err != nil {
		t.Fatalf("the first booking was refused: %v", err)
	}

	_, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 43,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Errorf("error = %v, want ErrSlotUnavailable", err)
	}
}

// The race the EXCLUDE constraint decides. The loser must receive the SAME answer a
// stale page gets: to the client the two events are identical, and a constraint
// violation reaching the handler would be a 500 for an ordinary outcome.
func TestALostRaceLooksExactlyLikeAStalePage(t *testing.T) {
	repo := newFakeRepo()
	mentor := bookableMentor(t, repo)
	// The engine still offers the slot; only the write refuses, as it would when another
	// request committed between the two.
	repo.createBookingErr = ErrSlotUnavailable

	_, err := bookingService(repo, nil).Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Errorf("error = %v, want ErrSlotUnavailable", err)
	}
}

func TestBookingRefusesAPausedOrUnapprovedMentor(t *testing.T) {
	t.Run("paused", func(t *testing.T) {
		repo := newFakeRepo()
		mentor := bookableMentor(t, repo)
		if _, err := repo.SetPaused(context.Background(), mentor.UserID, true); err != nil {
			t.Fatalf("SetPaused: %v", err)
		}

		_, err := bookingService(repo, nil).Book(context.Background(), BookingInput{
			MentorSlug: mentor.Slug, SeekerUserID: 42,
			StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
		})
		if !errors.Is(err, ErrProfileNotFound) {
			t.Errorf("error = %v, want ErrProfileNotFound — a paused mentor is not there", err)
		}
	})

	t.Run("never approved", func(t *testing.T) {
		repo := newFakeRepo()
		in := validInput()
		profile, err := repo.CreateProfile(context.Background(), in)
		if err != nil {
			t.Fatalf("CreateProfile: %v", err)
		}
		rule, err := NewWeeklyRule(time.Tuesday, mustTimeOfDay(t, 18, 0), mustTimeOfDay(t, 22, 0))
		if err != nil {
			t.Fatalf("NewWeeklyRule: %v", err)
		}
		repo.availability[profile.ID] = []Rule{rule}

		_, err = bookingService(repo, nil).Book(context.Background(), BookingInput{
			MentorSlug: profile.Slug, SeekerUserID: 42,
			StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
		})
		if !errors.Is(err, ErrProfileNotFound) {
			t.Errorf("error = %v, want ErrProfileNotFound", err)
		}
	})
}

func TestAMentorCannotBookThemselves(t *testing.T) {
	repo := newFakeRepo()
	mentor := bookableMentor(t, repo)

	_, err := bookingService(repo, nil).Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: mentor.UserID,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if !errors.Is(err, ErrCannotBookYourself) {
		t.Errorf("error = %v, want ErrCannotBookYourself", err)
	}
}

func TestBookingRefusesAnUnauthenticatedSeeker(t *testing.T) {
	repo := newFakeRepo()
	mentor := bookableMentor(t, repo)

	_, err := bookingService(repo, nil).Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 0,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("error = %v, want ErrNotAuthenticated", err)
	}
}

// A booking that succeeded must not be undone by a channel that failed. The seeker has
// the hour; the confirmation can be resent.
func TestAFailedConfirmationDoesNotUndoTheBooking(t *testing.T) {
	repo := newFakeRepo()
	mentor := bookableMentor(t, repo)
	notifier := &fakeNotifier{err: errors.New("smtp is down")}

	booking, err := bookingService(repo, notifier).Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v — a delivery failure must not fail the booking", err)
	}
	if booking.Status != BookingConfirmed {
		t.Errorf("status = %q, want confirmed", booking.Status)
	}
}

// A mentor with no calendar linker configured at all books byte-for-byte identically to
// today — the regression guard for the whole feature's overwhelming common case.
func TestBookingAnUnconnectedMentorIsUnchanged(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	svc := bookingService(repo, notifier)

	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if booking.MeetingURL != mentor.MeetingURL {
		t.Errorf("meeting link = %q, want the mentor's static link %q", booking.MeetingURL, mentor.MeetingURL)
	}
	if booking.GoogleEventID != "" {
		t.Errorf("GoogleEventID = %q, want empty", booking.GoogleEventID)
	}
}

// A mentor with a calendar linker configured but no actual grant (the ordinary case even
// once the feature ships, until a mentor opts in) also books unchanged: CreateMeetEvent
// answers ErrCalendarNotConnected and Book() leaves the static snapshot alone.
func TestBookingAMentorWithNoGrantIsUnchanged(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	linker := &fakeCalendarLinker{err: ErrCalendarNotConnected}
	svc := bookingServiceWithCalendar(repo, notifier, linker)

	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if booking.MeetingURL != mentor.MeetingURL {
		t.Errorf("meeting link = %q, want the mentor's static link", booking.MeetingURL)
	}
	if linker.calls != 1 {
		t.Errorf("CreateMeetEvent called %d times, want 1", linker.calls)
	}
}

// A connected mentor's booking carries the calendar's own Meet link and event id — not
// the mentor's static one — and the notifier receives that same link.
func TestBookingAConnectedMentorUsesTheCalendarLink(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	linker := &fakeCalendarLinker{eventID: "evt-1", meetLink: "https://meet.google.com/xyz-abcd-efg"}
	svc := bookingServiceWithCalendar(repo, notifier, linker)

	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if booking.MeetingURL != "https://meet.google.com/xyz-abcd-efg" {
		t.Errorf("meeting link = %q, want the calendar's own link", booking.MeetingURL)
	}
	if booking.GoogleEventID != "evt-1" {
		t.Errorf("GoogleEventID = %q, want evt-1", booking.GoogleEventID)
	}
	if len(notifier.confirmed) != 1 || notifier.confirmed[0].MeetingURL != booking.MeetingURL {
		t.Errorf("the notifier did not receive the calendar's link")
	}
	if got := repo.setBookingCalendarEvent[booking.ID]; got != [2]string{booking.MeetingURL, "evt-1"} {
		t.Errorf("SetBookingCalendarEvent recorded %v", got)
	}
}

// A CreateMeetEvent failure that is NOT ErrCalendarNotConnected still returns a confirmed
// booking, with an EMPTY link rather than the mentor's stale static one — the explicit
// decision this feature's design settled on over silently falling back.
func TestBookingCalendarFailureLeavesTheLinkEmptyButStillBooks(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	linker := &fakeCalendarLinker{err: errors.New("google: rate limited")}
	svc := bookingServiceWithCalendar(repo, notifier, linker)

	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v — a calendar failure must not fail the booking", err)
	}
	if booking.Status != BookingConfirmed {
		t.Errorf("status = %q, want confirmed", booking.Status)
	}
	if booking.MeetingURL != "" {
		t.Errorf("meeting link = %q, want empty", booking.MeetingURL)
	}
	if repo.needsReconsent[mentor.UserID] {
		t.Error("a non-revocation failure must not mark needs_reconsent")
	}
}

// A revocation-shaped calendar failure additionally marks the mentor's grant for
// re-consent, the same mechanism gmailsync's own sync worker uses for the read-side
// grants.
func TestBookingCalendarRevocationMarksNeedsReconsent(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	linker := &fakeCalendarLinker{err: &gmailsync.APIError{Op: "mentor calendar: create event", StatusCode: 401, Status: "401 Unauthorized"}}
	svc := bookingServiceWithCalendar(repo, notifier, linker)

	if _, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	}); err != nil {
		t.Fatalf("Book: %v", err)
	}
	if !repo.needsReconsent[mentor.UserID] {
		t.Error("a revocation-shaped failure must mark needs_reconsent")
	}
}

// Cancelling a booking with an event id best-effort deletes it; the mentor's own user id
// is what DeleteMeetEvent is called with, since the grant lives on the mentor's account.
func TestCancellingABookingWithAnEventDeletesIt(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	linker := &fakeCalendarLinker{eventID: "evt-1", meetLink: "https://meet.google.com/xyz"}
	svc := bookingServiceWithCalendar(repo, notifier, linker)

	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	if _, err := svc.Cancel(context.Background(), booking.ID, 42, "changed my mind"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if linker.deletedEventID != "evt-1" {
		t.Errorf("DeleteMeetEvent called with %q, want evt-1", linker.deletedEventID)
	}
}

// A DeleteMeetEvent failure must not fail the cancellation: the session is already off
// and both parties already told, and a stray event on the mentor's own calendar costs
// nobody anything a retried cancel could fix.
func TestCancellingSurvivesADeleteMeetEventFailure(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	linker := &fakeCalendarLinker{eventID: "evt-1", meetLink: "https://meet.google.com/xyz", deleteErr: errors.New("google: unavailable")}
	svc := bookingServiceWithCalendar(repo, notifier, linker)

	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	cancelled, err := svc.Cancel(context.Background(), booking.ID, 42, "changed my mind")
	if err != nil {
		t.Fatalf("Cancel: %v — a delete failure must not fail the cancellation", err)
	}
	if cancelled.Status != BookingCancelled {
		t.Errorf("status = %q, want cancelled", cancelled.Status)
	}
}

func TestCancellationTellsTheOtherPartyAndFreesTheSlot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		canceller func(mentor Profile) int64
		wantBy    CancelledBy
	}{
		{"the seeker cancels", func(Profile) int64 { return 42 }, CancelledBySeeker},
		{"the mentor cancels", func(m Profile) int64 { return m.UserID }, CancelledByMentor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			notifier := &fakeNotifier{}
			mentor := bookableMentor(t, repo)
			svc := bookingService(repo, notifier)

			booking, err := svc.Book(context.Background(), BookingInput{
				MentorSlug: mentor.Slug, SeekerUserID: 42,
				StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
			})
			if err != nil {
				t.Fatalf("Book: %v", err)
			}

			cancelled, err := svc.Cancel(context.Background(), booking.ID, tc.canceller(mentor), "something came up")
			if err != nil {
				t.Fatalf("Cancel: %v", err)
			}
			if cancelled.Status != BookingCancelled {
				t.Errorf("status = %q, want cancelled", cancelled.Status)
			}
			if len(notifier.cancelled) != 1 {
				t.Fatalf("%d cancellations sent, want 1", len(notifier.cancelled))
			}
			if notifier.cancelledBy[0] != tc.wantBy {
				t.Errorf("notified as cancelled by %q, want %q", notifier.cancelledBy[0], tc.wantBy)
			}

			// The freed hour is bookable again.
			if _, err := svc.Book(context.Background(), BookingInput{
				MentorSlug: mentor.Slug, SeekerUserID: 43,
				StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
			}); err != nil {
				t.Errorf("the freed slot was not bookable: %v", err)
			}
		})
	}
}

func TestAStrangerCannotCancel(t *testing.T) {
	repo := newFakeRepo()
	notifier := &fakeNotifier{}
	mentor := bookableMentor(t, repo)
	svc := bookingService(repo, notifier)

	booking, err := svc.Book(context.Background(), BookingInput{
		MentorSlug: mentor.Slug, SeekerUserID: 42,
		StartsAt: tuesdayAt(t, 18), SeekerTimezone: "Asia/Tokyo",
	})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}

	// The same answer a booking that does not exist gets: a stranger must not learn that
	// somebody else's session is real.
	if _, err := svc.Cancel(context.Background(), booking.ID, 4242, ""); !errors.Is(err, ErrBookingNotFound) {
		t.Errorf("error = %v, want ErrBookingNotFound", err)
	}
	if len(notifier.cancelled) != 0 {
		t.Error("a stranger's failed cancellation notified somebody")
	}
	if _, err := svc.Cancel(context.Background(), uuid.New(), 42, ""); !errors.Is(err, ErrBookingNotFound) {
		t.Errorf("cancelling an unknown booking: error = %v, want ErrBookingNotFound", err)
	}
}

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
