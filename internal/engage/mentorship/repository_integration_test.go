//go:build integration

// Integration tests for QueriesRepository — the seam between the SQL and the domain type.
//
// This is the gap the other two test layers leave open. The unit tests drive a FAKE
// repository, which fills the domain struct itself and so passes whatever the real mapping
// does; and internal/platform/db's tests call the generated query directly, which proves
// the columns come back but not that anything reads them. A field the query selects and
// the mapping forgets is invisible to both, and that is exactly the defect this file was
// opened for: a seeker's session list arrived naming nobody, because the loop copied the
// mentor's slug and dropped their headline.
//
// Run with: go test -tags=integration ./internal/engage/mentorship/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package mentorship

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

// A session list has to say WHO the session is with. Asserted through the repository
// rather than through the query, because the query was right the whole time.
func TestListBookingsBySeekerCarriesTheMentorsIdentity(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := db.New(pool)
	repo := NewQueriesRepository(queries, pool)

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('repoco', 'Repo Co') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	mentorUser := seedUser(t, pool, "mentor-repo@example.test")
	seeker := seedUser(t, pool, "seeker-repo@example.test")

	mentor, err := queries.CreateMentorProfile(ctx, db.CreateMentorProfileParams{
		UserID: mentorUser, CompanySlug: pgtype.Text{String: "repoco", Valid: true}, Slug: "repo-mentor",
		DisplayName: "Dana R.", Headline: "Principal Engineer", Bio: "",
		Topics: []string{"career"}, Languages: []string{"en"},
		Timezone: "Europe/Berlin", SessionDurationMin: 60,
		MinNoticeMin: 120, HorizonDays: 30,
		MeetingUrl: "https://meet.example.test/dana",
	})
	if err != nil {
		t.Fatalf("CreateMentorProfile: %v", err)
	}

	start := time.Now().Add(48 * time.Hour)
	if _, err := queries.CreateMentorBooking(ctx, db.CreateMentorBookingParams{
		MentorID: mentor.ID, SeekerUserID: seeker,
		StartsAt:       pgtype.Timestamptz{Time: start, Valid: true},
		EndsAt:         pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		SeekerTimezone: "Asia/Tokyo", MeetingUrl: "https://meet.example.test/dana",
	}); err != nil {
		t.Fatalf("CreateMentorBooking: %v", err)
	}

	bookings, err := repo.ListBookingsBySeeker(ctx, seeker, 10)
	if err != nil {
		t.Fatalf("ListBookingsBySeeker: %v", err)
	}
	if len(bookings) != 1 {
		t.Fatalf("got %d bookings, want 1", len(bookings))
	}

	// Two separate ways for a row to arrive anonymous. The slug was always copied; the
	// headline was the one that went missing, and it is what a person actually reads.
	if bookings[0].MentorSlug != "repo-mentor" {
		t.Errorf("MentorSlug = %q, want repo-mentor", bookings[0].MentorSlug)
	}
	if bookings[0].MentorHeadline != "Principal Engineer" {
		t.Errorf("MentorHeadline = %q, want Principal Engineer — the list names nobody without it",
			bookings[0].MentorHeadline)
	}
}

// ListBusy has always unioned bookings and synced-calendar intervals into one flat set —
// right for the public slot engine, which only needs to know time is occupied. The new
// own-calendar breakdown needs to tell them apart, so ListBusyByKind must return the same
// two sources separately rather than merged.
func TestListBusyByKindSeparatesBookingsFromSyncedIntervals(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := db.New(pool)
	repo := NewQueriesRepository(queries, pool)

	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('busykindco', 'Busy Kind Co') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	mentorUser := seedUser(t, pool, "mentor-busykind@example.test")
	seeker := seedUser(t, pool, "seeker-busykind@example.test")

	mentor, err := queries.CreateMentorProfile(ctx, db.CreateMentorProfileParams{
		UserID: mentorUser, CompanySlug: pgtype.Text{String: "busykindco", Valid: true}, Slug: "busykind-mentor",
		DisplayName: "Kim B.", Headline: "Staff Engineer", Bio: "",
		Topics: []string{"career"}, Languages: []string{"en"},
		Timezone: "Europe/Berlin", SessionDurationMin: 60,
		MinNoticeMin: 120, HorizonDays: 30,
		MeetingUrl: "https://meet.example.test/kim",
	})
	if err != nil {
		t.Fatalf("CreateMentorProfile: %v", err)
	}

	bookingStart := time.Now().Add(24 * time.Hour).Truncate(time.Minute)
	if _, err := queries.CreateMentorBooking(ctx, db.CreateMentorBookingParams{
		MentorID: mentor.ID, SeekerUserID: seeker,
		StartsAt:       pgtype.Timestamptz{Time: bookingStart, Valid: true},
		EndsAt:         pgtype.Timestamptz{Time: bookingStart.Add(time.Hour), Valid: true},
		SeekerTimezone: "Asia/Tokyo", MeetingUrl: "https://meet.example.test/kim",
	}); err != nil {
		t.Fatalf("CreateMentorBooking: %v", err)
	}

	syncedStart := time.Now().Add(72 * time.Hour).Truncate(time.Minute)
	if err := queries.UpsertMentorBusyInterval(ctx, db.UpsertMentorBusyIntervalParams{
		MentorID:   mentor.ID,
		StartsAt:   pgtype.Timestamptz{Time: syncedStart, Valid: true},
		EndsAt:     pgtype.Timestamptz{Time: syncedStart.Add(30 * time.Minute), Valid: true},
		ExternalID: "busykind-1",
	}); err != nil {
		t.Fatalf("UpsertMentorBusyInterval: %v", err)
	}

	from := time.Now()
	to := from.Add(7 * 24 * time.Hour)
	booked, busy, err := repo.ListBusyByKind(ctx, mentor.ID, from, to)
	if err != nil {
		t.Fatalf("ListBusyByKind: %v", err)
	}

	if len(booked) != 1 {
		t.Fatalf("got %d booked intervals, want 1", len(booked))
	}
	if !booked[0].Start.Equal(bookingStart) || !booked[0].End.Equal(bookingStart.Add(time.Hour)) {
		t.Errorf("booked[0] = %+v, want %s–%s", booked[0], bookingStart, bookingStart.Add(time.Hour))
	}

	if len(busy) != 1 {
		t.Fatalf("got %d busy intervals, want 1", len(busy))
	}
	if !busy[0].Start.Equal(syncedStart) || !busy[0].End.Equal(syncedStart.Add(30*time.Minute)) {
		t.Errorf("busy[0] = %+v, want %s–%s", busy[0], syncedStart, syncedStart.Add(30*time.Minute))
	}
}
