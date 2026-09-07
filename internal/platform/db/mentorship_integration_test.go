//go:build integration

// Integration tests for the mentorship query semantics that only a real Postgres can
// answer: the EXCLUDE constraint that guarantees two confirmed bookings for one mentor
// cannot overlap, the three guards CancelMentorBooking packs into one statement, the
// reminder claim's idempotency, and the directory's "NULL means unfiltered" filters.
//
// The non-overlap guarantee in particular exists BECAUSE it is not in Go. Testing it
// anywhere but here would test the thing that does not hold it.
//
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func seedMentorshipUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func seedMentorshipCompany(t *testing.T, pool *pgxpool.Pool, slug string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO companies (slug, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		slug, slug); err != nil {
		t.Fatalf("seed company %s: %v", slug, err)
	}
}

func seedMentor(t *testing.T, q *Queries, userID int64, company, slug string) Mentor {
	t.Helper()
	mentor, err := q.CreateMentorProfile(context.Background(), CreateMentorProfileParams{
		UserID:             userID,
		CompanySlug:        company,
		Slug:               slug,
		Headline:           "Senior Engineer",
		Bio:                "",
		Topics:             []string{"career", "system-design"},
		Languages:          []string{"en"},
		Timezone:           "Europe/Berlin",
		SessionDurationMin: 60,
		MinNoticeMin:       120,
		HorizonDays:        30,
		MeetingUrl:         "https://meet.example.test/" + slug,
	})
	if err != nil {
		t.Fatalf("CreateMentorProfile(%s): %v", slug, err)
	}
	return mentor
}

func approve(t *testing.T, q *Queries, mentorID, moderator int64) Mentor {
	t.Helper()
	got, err := q.DecideMentorProfile(context.Background(), DecideMentorProfileParams{
		ID: mentorID, Status: "approved", DecidedBy: pgtype.Int8{Int64: moderator, Valid: true},
	})
	if err != nil {
		t.Fatalf("DecideMentorProfile: %v", err)
	}
	return got
}

func book(t *testing.T, q *Queries, mentorID, seeker int64, start time.Time) (MentorBooking, error) {
	t.Helper()
	return q.CreateMentorBooking(context.Background(), CreateMentorBookingParams{
		MentorID:       mentorID,
		SeekerUserID:   seeker,
		StartsAt:       pgtype.Timestamptz{Time: start, Valid: true},
		EndsAt:         pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		SeekerTimezone: "Asia/Tokyo",
		MeetingUrl:     "https://meet.example.test/x",
	})
}

// The whole reason the constraint exists: two requests for one slot must leave exactly
// one booking, and the loser must be told the slot is gone rather than crash. cal.com
// re-checks in application code and leaves this window open.
func TestMentorBookingsCannotOverlap(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)

	seedMentorshipCompany(t, pool, "acme")
	mentorUser := seedMentorshipUser(t, pool, "mentor-overlap@example.test")
	alice := seedMentorshipUser(t, pool, "alice-overlap@example.test")
	bob := seedMentorshipUser(t, pool, "bob-overlap@example.test")
	mentor := seedMentor(t, q, mentorUser, "acme", "overlap-mentor")

	at := func(hour int) time.Time {
		return time.Date(2026, time.September, 15, hour, 0, 0, 0, time.UTC)
	}

	if _, err := book(t, q, mentor.ID, alice, at(18)); err != nil {
		t.Fatalf("the first booking was refused: %v", err)
	}

	t.Run("the same slot is refused", func(t *testing.T) {
		if _, err := book(t, q, mentor.ID, bob, at(18)); err == nil {
			t.Error("a second booking for the same slot was accepted")
		}
	})

	t.Run("a straddling slot is refused", func(t *testing.T) {
		_, err := q.CreateMentorBooking(context.Background(), CreateMentorBookingParams{
			MentorID:       mentor.ID,
			SeekerUserID:   bob,
			StartsAt:       pgtype.Timestamptz{Time: at(18).Add(30 * time.Minute), Valid: true},
			EndsAt:         pgtype.Timestamptz{Time: at(19).Add(30 * time.Minute), Valid: true},
			SeekerTimezone: "Asia/Tokyo",
			MeetingUrl:     "https://meet.example.test/x",
		})
		if err == nil {
			t.Error("an overlapping booking was accepted")
		}
	})

	// Half-open bounds, the same rule Interval.Overlaps holds in Go. If these two ever
	// disagree, the engine offers a slot the insert rejects.
	t.Run("a back-to-back slot is accepted", func(t *testing.T) {
		if _, err := book(t, q, mentor.ID, bob, at(19)); err != nil {
			t.Errorf("a back-to-back booking was refused: %v", err)
		}
	})

	t.Run("another mentor may hold the same hour", func(t *testing.T) {
		otherUser := seedMentorshipUser(t, pool, "mentor2-overlap@example.test")
		other := seedMentor(t, q, otherUser, "acme", "overlap-mentor-2")
		if _, err := book(t, q, other.ID, alice, at(18)); err != nil {
			t.Errorf("a different mentor's booking at the same hour was refused: %v", err)
		}
	})
}

// The WHERE predicate on the constraint, which is what makes cancellation mean anything.
func TestACancelledBookingFreesItsSlot(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedMentorshipCompany(t, pool, "acme")
	mentorUser := seedMentorshipUser(t, pool, "mentor-free@example.test")
	alice := seedMentorshipUser(t, pool, "alice-free@example.test")
	bob := seedMentorshipUser(t, pool, "bob-free@example.test")
	mentor := seedMentor(t, q, mentorUser, "acme", "free-mentor")

	// Far enough ahead that CancelMentorBooking's starts_at > now() guard passes.
	start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Hour)

	first, err := book(t, q, mentor.ID, alice, start)
	if err != nil {
		t.Fatalf("first booking: %v", err)
	}
	if _, err := book(t, q, mentor.ID, bob, start); err == nil {
		t.Fatal("the slot was bookable twice before cancelling")
	}

	cancelled, err := q.CancelMentorBooking(ctx, CancelMentorBookingParams{
		ID: first.ID, CancelledBy: alice, CancelReason: "changed my mind",
	})
	if err != nil {
		t.Fatalf("CancelMentorBooking: %v", err)
	}
	if cancelled.Status != "cancelled" || !cancelled.CancelledAt.Valid {
		t.Errorf("status=%q cancelled_at valid=%v, want cancelled with a time",
			cancelled.Status, cancelled.CancelledAt.Valid)
	}

	if _, err := book(t, q, mentor.ID, bob, start); err != nil {
		t.Errorf("the freed slot was not bookable again: %v", err)
	}
}

// Three guards live in that one UPDATE, and a stranger failing must be indistinguishable
// from a booking that does not exist.
func TestCancelMentorBookingGuards(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedMentorshipCompany(t, pool, "acme")
	mentorUser := seedMentorshipUser(t, pool, "mentor-guard@example.test")
	seeker := seedMentorshipUser(t, pool, "seeker-guard@example.test")
	stranger := seedMentorshipUser(t, pool, "stranger-guard@example.test")
	mentor := seedMentor(t, q, mentorUser, "acme", "guard-mentor")

	future := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Hour)

	t.Run("a stranger cancels nothing", func(t *testing.T) {
		booking, err := book(t, q, mentor.ID, seeker, future)
		if err != nil {
			t.Fatalf("book: %v", err)
		}
		if _, err := q.CancelMentorBooking(ctx, CancelMentorBookingParams{
			ID: booking.ID, CancelledBy: stranger,
		}); err == nil {
			t.Error("a stranger cancelled somebody else's session")
		}
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM mentor_bookings WHERE id = $1`, booking.ID).Scan(&status); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if status != "confirmed" {
			t.Errorf("status = %q after a stranger's attempt, want confirmed", status)
		}
	})

	t.Run("the mentor may cancel", func(t *testing.T) {
		booking, err := book(t, q, mentor.ID, seeker, future.Add(2*time.Hour))
		if err != nil {
			t.Fatalf("book: %v", err)
		}
		if _, err := q.CancelMentorBooking(ctx, CancelMentorBookingParams{
			ID: booking.ID, CancelledBy: mentorUser,
		}); err != nil {
			t.Errorf("the mentor could not cancel their own session: %v", err)
		}
	})

	t.Run("a second cancellation matches nothing", func(t *testing.T) {
		booking, err := book(t, q, mentor.ID, seeker, future.Add(4*time.Hour))
		if err != nil {
			t.Fatalf("book: %v", err)
		}
		if _, err := q.CancelMentorBooking(ctx, CancelMentorBookingParams{
			ID: booking.ID, CancelledBy: seeker,
		}); err != nil {
			t.Fatalf("first cancellation: %v", err)
		}
		if _, err := q.CancelMentorBooking(ctx, CancelMentorBookingParams{
			ID: booking.ID, CancelledBy: seeker,
		}); err == nil {
			t.Error("a cancelled session was cancelled again")
		}
	})

	t.Run("a session already begun cannot be cancelled", func(t *testing.T) {
		past := time.Now().UTC().Add(-2 * time.Hour)
		booking, err := book(t, q, mentor.ID, seeker, past)
		if err != nil {
			t.Fatalf("book: %v", err)
		}
		if _, err := q.CancelMentorBooking(ctx, CancelMentorBookingParams{
			ID: booking.ID, CancelledBy: seeker,
		}); err == nil {
			t.Error("a session in the past was cancelled")
		}
	})
}

// The reminder claim. RecordReminderSent is the claim, not the record: a caller that
// sends before checking the row count sends twice.
func TestReminderClaimIsIdempotent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedMentorshipCompany(t, pool, "acme")
	mentorUser := seedMentorshipUser(t, pool, "mentor-remind@example.test")
	seeker := seedMentorshipUser(t, pool, "seeker-remind@example.test")
	mentor := seedMentor(t, q, mentorUser, "acme", "remind-mentor")
	approve(t, q, mentor.ID, mentorUser)

	// Inside the 24h window, outside the 1h one.
	soon := time.Now().UTC().Add(3 * time.Hour)
	booking, err := book(t, q, mentor.ID, seeker, soon)
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	due, err := q.ListBookingsDueForReminder(ctx, ListBookingsDueForReminderParams{
		OffsetMinutes: 1440, RowLimit: 100,
	})
	if err != nil {
		t.Fatalf("ListBookingsDueForReminder: %v", err)
	}
	if !containsBooking(due, booking.ID) {
		t.Fatal("a session three hours away is not due for its 24-hour reminder")
	}

	rows, err := q.RecordReminderSent(ctx, RecordReminderSentParams{
		BookingID: booking.ID, OffsetMinutes: 1440,
	})
	if err != nil {
		t.Fatalf("RecordReminderSent: %v", err)
	}
	if rows != 1 {
		t.Fatalf("first claim affected %d rows, want 1", rows)
	}

	rows, err = q.RecordReminderSent(ctx, RecordReminderSentParams{
		BookingID: booking.ID, OffsetMinutes: 1440,
	})
	if err != nil {
		t.Fatalf("second RecordReminderSent: %v", err)
	}
	if rows != 0 {
		t.Errorf("second claim affected %d rows, want 0 — it would have sent twice", rows)
	}

	due, err = q.ListBookingsDueForReminder(ctx, ListBookingsDueForReminderParams{
		OffsetMinutes: 1440, RowLimit: 100,
	})
	if err != nil {
		t.Fatalf("ListBookingsDueForReminder after claim: %v", err)
	}
	if containsBooking(due, booking.ID) {
		t.Error("a reminded session is still listed as due")
	}
}

// A run that arrives after the session has started must send nothing: "your session
// starts in an hour", delivered afterwards, is worse than silence.
func TestAMissedReminderDoesNotFireLate(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedMentorshipCompany(t, pool, "acme")
	mentorUser := seedMentorshipUser(t, pool, "mentor-late@example.test")
	seeker := seedMentorshipUser(t, pool, "seeker-late@example.test")
	mentor := seedMentor(t, q, mentorUser, "acme", "late-mentor")

	booking, err := book(t, q, mentor.ID, seeker, time.Now().UTC().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	due, err := q.ListBookingsDueForReminder(ctx, ListBookingsDueForReminderParams{
		OffsetMinutes: 60, RowLimit: 100,
	})
	if err != nil {
		t.Fatalf("ListBookingsDueForReminder: %v", err)
	}
	if containsBooking(due, booking.ID) {
		t.Error("a session that already started is listed as due for a reminder")
	}
}

// A cancelled session receives no further reminders.
func TestACancelledBookingIsNotDueForReminders(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedMentorshipCompany(t, pool, "acme")
	mentorUser := seedMentorshipUser(t, pool, "mentor-cancel-remind@example.test")
	seeker := seedMentorshipUser(t, pool, "seeker-cancel-remind@example.test")
	mentor := seedMentor(t, q, mentorUser, "acme", "cancel-remind-mentor")

	booking, err := book(t, q, mentor.ID, seeker, time.Now().UTC().Add(3*time.Hour))
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	if _, err := q.CancelMentorBooking(ctx, CancelMentorBookingParams{
		ID: booking.ID, CancelledBy: seeker,
	}); err != nil {
		t.Fatalf("CancelMentorBooking: %v", err)
	}

	due, err := q.ListBookingsDueForReminder(ctx, ListBookingsDueForReminderParams{
		OffsetMinutes: 1440, RowLimit: 100,
	})
	if err != nil {
		t.Fatalf("ListBookingsDueForReminder: %v", err)
	}
	if containsBooking(due, booking.ID) {
		t.Error("a cancelled session is still due for a reminder")
	}
}

// The directory's publication predicate, which must be identical to the one the
// vacancy-page check uses — two readers disagreeing about "has a mentor" is a page
// offering a link to an empty list.
func TestOnlyApprovedUnpausedMentorsArePublished(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedMentorshipCompany(t, pool, "publishco")
	moderator := seedMentorshipUser(t, pool, "moderator-pub@example.test")
	pendingUser := seedMentorshipUser(t, pool, "pending-pub@example.test")
	approvedUser := seedMentorshipUser(t, pool, "approved-pub@example.test")
	pausedUser := seedMentorshipUser(t, pool, "paused-pub@example.test")

	seedMentor(t, q, pendingUser, "publishco", "pending-one")
	approvedMentor := seedMentor(t, q, approvedUser, "publishco", "approved-one")
	pausedMentor := seedMentor(t, q, pausedUser, "publishco", "paused-one")
	approve(t, q, approvedMentor.ID, moderator)
	approve(t, q, pausedMentor.ID, moderator)
	if _, err := q.SetMentorPaused(ctx, SetMentorPausedParams{
		ID: pausedMentor.ID, UserID: pausedUser, Paused: true,
	}); err != nil {
		t.Fatalf("SetMentorPaused: %v", err)
	}

	listed, err := q.ListPublishedMentors(ctx, ListPublishedMentorsParams{
		CompanySlug: pgtype.Text{String: "publishco", Valid: true},
		RowLimit:    50,
	})
	if err != nil {
		t.Fatalf("ListPublishedMentors: %v", err)
	}
	if len(listed) != 1 || listed[0].Slug != "approved-one" {
		var slugs []string
		for _, m := range listed {
			slugs = append(slugs, m.Slug)
		}
		t.Fatalf("directory holds %v, want only approved-one", slugs)
	}

	t.Run("the vacancy-page check agrees", func(t *testing.T) {
		has, err := q.CompanyHasPublishedMentor(ctx, "publishco")
		if err != nil {
			t.Fatalf("CompanyHasPublishedMentor: %v", err)
		}
		if !has {
			t.Error("the company has one published mentor but the check says no")
		}
	})

	t.Run("a pending profile is not publicly readable", func(t *testing.T) {
		if _, err := q.GetPublishedMentorBySlug(ctx, "pending-one"); err == nil {
			t.Error("a pending profile answered the public read")
		}
	})

	t.Run("a paused profile is not publicly readable", func(t *testing.T) {
		if _, err := q.GetPublishedMentorBySlug(ctx, "paused-one"); err == nil {
			t.Error("a paused profile answered the public read")
		}
	})

	t.Run("the owner still sees their own pending profile", func(t *testing.T) {
		got, err := q.GetMentorByUserID(ctx, pendingUser)
		if err != nil {
			t.Fatalf("GetMentorByUserID: %v", err)
		}
		if got.Status != "pending" {
			t.Errorf("status = %q, want pending", got.Status)
		}
	})
}

// Every directory filter is "NULL means unfiltered". A filter that silently matched
// nothing when absent would empty the directory; one that silently matched everything
// when present would widen a search the caller narrowed.
func TestDirectoryFiltersTreatNullAsUnfiltered(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedMentorshipCompany(t, pool, "filterco")
	moderator := seedMentorshipUser(t, pool, "moderator-filter@example.test")
	user := seedMentorshipUser(t, pool, "mentor-filter@example.test")
	mentor := seedMentor(t, q, user, "filterco", "filter-mentor")
	approve(t, q, mentor.ID, moderator)

	for _, tc := range []struct {
		name  string
		arg   ListPublishedMentorsParams
		wantN int
	}{
		{"no filters at all", ListPublishedMentorsParams{RowLimit: 50}, 1},
		{"matching topic", ListPublishedMentorsParams{
			Topic: pgtype.Text{String: "career", Valid: true}, RowLimit: 50}, 1},
		{"non-matching topic", ListPublishedMentorsParams{
			Topic: pgtype.Text{String: "underwater-basketry", Valid: true}, RowLimit: 50}, 0},
		{"matching language", ListPublishedMentorsParams{
			Language: pgtype.Text{String: "en", Valid: true}, RowLimit: 50}, 1},
		{"non-matching language", ListPublishedMentorsParams{
			Language: pgtype.Text{String: "xx", Valid: true}, RowLimit: 50}, 0},
		{"non-matching company", ListPublishedMentorsParams{
			CompanySlug: pgtype.Text{String: "nobody", Valid: true}, RowLimit: 50}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := q.ListPublishedMentors(ctx, tc.arg)
			if err != nil {
				t.Fatalf("ListPublishedMentors: %v", err)
			}
			found := 0
			for _, m := range got {
				if m.Slug == "filter-mentor" {
					found++
				}
			}
			if found != tc.wantN {
				t.Errorf("found the mentor %d times, want %d", found, tc.wantN)
			}
		})
	}
}

// One profile per account, and one review per booking — both are constraints rather than
// service checks, which is what makes a second write an update or a refusal by
// construction.
func TestMentorshipUniquenessConstraints(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	seedMentorshipCompany(t, pool, "uniqueco")
	mentorUser := seedMentorshipUser(t, pool, "mentor-unique@example.test")
	seeker := seedMentorshipUser(t, pool, "seeker-unique@example.test")
	mentor := seedMentor(t, q, mentorUser, "uniqueco", "unique-mentor")

	t.Run("a second profile for one account is refused", func(t *testing.T) {
		if _, err := q.CreateMentorProfile(ctx, CreateMentorProfileParams{
			UserID: mentorUser, CompanySlug: "uniqueco", Slug: "unique-mentor-again",
			Headline: "Also me", Timezone: "Europe/Berlin",
			SessionDurationMin: 30, HorizonDays: 30, MeetingUrl: "https://meet.example.test/z",
		}); err == nil {
			t.Error("an account holds two mentor profiles")
		}
	})

	t.Run("a profile naming an unknown company is refused", func(t *testing.T) {
		other := seedMentorshipUser(t, pool, "mentor-nocompany@example.test")
		if _, err := q.CreateMentorProfile(ctx, CreateMentorProfileParams{
			UserID: other, CompanySlug: "no-such-company", Slug: "orphan-mentor",
			Headline: "Nobody", Timezone: "Europe/Berlin",
			SessionDurationMin: 30, HorizonDays: 30, MeetingUrl: "https://meet.example.test/z",
		}); err == nil {
			t.Error("a profile was created for a company the catalogue does not carry")
		}
	})

	t.Run("a second review updates rather than duplicates", func(t *testing.T) {
		booking, err := book(t, q, mentor.ID, seeker, time.Now().UTC().Add(96*time.Hour))
		if err != nil {
			t.Fatalf("book: %v", err)
		}
		for _, rating := range []int16{5, 2} {
			if _, err := q.UpsertMentorReview(ctx, UpsertMentorReviewParams{
				BookingID: booking.ID, MentorID: mentor.ID, SeekerUserID: seeker,
				Rating: rating, Comment: "",
			}); err != nil {
				t.Fatalf("UpsertMentorReview(%d): %v", rating, err)
			}
		}

		summary, err := q.GetMentorReviewSummary(ctx, mentor.ID)
		if err != nil {
			t.Fatalf("GetMentorReviewSummary: %v", err)
		}
		if summary.RatingCount != 1 {
			t.Errorf("rating count = %d after two submissions, want 1", summary.RatingCount)
		}
	})
}

func containsBooking(rows []ListBookingsDueForReminderRow, id pgtype.UUID) bool {
	for _, r := range rows {
		if r.ID == id {
			return true
		}
	}
	return false
}
