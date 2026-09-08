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
		UserID: mentorUser, CompanySlug: "repoco", Slug: "repo-mentor",
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
