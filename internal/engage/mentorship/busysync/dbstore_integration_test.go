//go:build integration

// Integration tests for DBStore — the seam between the SQL and this package's Store
// contract. Run with: go test -tags=integration ./internal/engage/mentorship/busysync/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package busysync

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func pgTimestamptz(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func seedApprovedMentor(t *testing.T, pool *pgxpool.Pool, queries *db.Queries, userID int64, slug string) int64 {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('busysyncco', 'Busysync Co') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed company: %v", err)
	}
	mentor, err := queries.CreateMentorProfile(ctx, db.CreateMentorProfileParams{
		UserID: userID, CompanySlug: "busysyncco", Slug: slug,
		DisplayName: "Test Mentor", Headline: "Engineer", Bio: "",
		Topics: []string{"career"}, Languages: []string{"en"},
		Timezone: "Europe/Berlin", SessionDurationMin: 60,
		MinNoticeMin: 120, HorizonDays: 30,
		MeetingUrl: "https://meet.example.test/" + slug,
	})
	if err != nil {
		t.Fatalf("CreateMentorProfile: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE mentors SET status = 'approved' WHERE id = $1`, mentor.ID); err != nil {
		t.Fatalf("approve mentor: %v", err)
	}
	return mentor.ID
}

// ListConnections must require every condition the design calls for at once: the
// explicit opt-in flag, the scope, a connected status, and an approved profile. Missing
// any one of them must exclude the mentor — the unrelated-grant case (scope without the
// flag) is the one this whole feature exists to get right.
func TestListConnectionsRequiresEveryCondition(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := db.New(pool)
	store := NewDBStore(queries, pool)

	seed := func(email, slug string, optedIn bool, scopes []string, status string, approve bool) int64 {
		uid := seedUser(t, pool, email)
		if _, err := pool.Exec(ctx,
			`INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status, scopes, mentor_busy_sync_opted_in)
			 VALUES ($1, '', 'enc', $2, $3, $4)`,
			uid, status, scopes, optedIn); err != nil {
			t.Fatalf("seed connection for %s: %v", email, err)
		}
		var mentorID int64
		if approve {
			mentorID = seedApprovedMentor(t, pool, queries, uid, slug)
		} else {
			mentor, err := queries.CreateMentorProfile(ctx, db.CreateMentorProfileParams{
				UserID: uid, CompanySlug: "busysyncco", Slug: slug,
				DisplayName: "Test Mentor", Headline: "Engineer", Bio: "",
				Topics: []string{"career"}, Languages: []string{"en"},
				Timezone: "Europe/Berlin", SessionDurationMin: 60,
				MinNoticeMin: 120, HorizonDays: 30,
				MeetingUrl: "https://meet.example.test/" + slug,
			})
			if err != nil {
				t.Fatalf("seed pending mentor for %s: %v", email, err)
			}
			mentorID = mentor.ID
		}
		return mentorID
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('busysyncco', 'Busysync Co') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	eligible := seed("eligible@example.test", "eligible", true, []string{gmailsync.CalendarScope}, "connected", true)
	seed("no-optin@example.test", "no-optin", false, []string{gmailsync.CalendarScope}, "connected", true)
	seed("no-scope@example.test", "no-scope", true, []string{}, "connected", true)
	seed("reconsent@example.test", "reconsent", true, []string{gmailsync.CalendarScope}, "needs_reconsent", true)
	seed("pending@example.test", "pending", true, []string{gmailsync.CalendarScope}, "connected", false)

	conns, err := store.ListConnections(ctx)
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("got %d connections, want exactly the one meeting every condition: %+v", len(conns), conns)
	}
	if conns[0].MentorID != eligible {
		t.Errorf("MentorID = %d, want %d", conns[0].MentorID, eligible)
	}
}

// ReplaceBusyWindow is a transactional replace, not a merge: a stale interval inside the
// window must be gone, and the new periods must be exactly what was passed, keyed so a
// re-sync of an unchanged interval updates rather than duplicates.
func TestReplaceBusyWindowReplacesRatherThanMerges(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	queries := db.New(pool)
	store := NewDBStore(queries, pool)

	uid := seedUser(t, pool, "replace@example.test")
	mentorID := seedApprovedMentor(t, pool, queries, uid, "replace-mentor")

	now := time.Now().Truncate(time.Second)
	windowEnd := now.AddDate(0, 0, busyWindowDays)
	stale := BusyPeriod{Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)}
	if err := store.ReplaceBusyWindow(ctx, mentorID, windowEnd, []BusyPeriod{stale}); err != nil {
		t.Fatalf("seed stale window: %v", err)
	}

	fresh := BusyPeriod{Start: now.Add(24 * time.Hour), End: now.Add(25 * time.Hour)}
	if err := store.ReplaceBusyWindow(ctx, mentorID, windowEnd, []BusyPeriod{fresh}); err != nil {
		t.Fatalf("ReplaceBusyWindow: %v", err)
	}

	rows, err := queries.ListMentorBusyIntervals(ctx, db.ListMentorBusyIntervalsParams{
		MentorID:    mentorID,
		WindowStart: pgTimestamptz(now.Add(-24 * time.Hour)),
		WindowEnd:   pgTimestamptz(windowEnd.Add(24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("ListMentorBusyIntervals: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d stored intervals, want exactly the fresh one (the stale one must be gone): %+v", len(rows), rows)
	}
	if !rows[0].StartsAt.Time.Equal(fresh.Start) {
		t.Errorf("StartsAt = %v, want %v", rows[0].StartsAt.Time, fresh.Start)
	}

	// A re-sync of the SAME interval must update, not duplicate — the whole point of
	// externalID being derived from the bounds.
	if err := store.ReplaceBusyWindow(ctx, mentorID, windowEnd, []BusyPeriod{fresh}); err != nil {
		t.Fatalf("re-sync ReplaceBusyWindow: %v", err)
	}
	rows, err = queries.ListMentorBusyIntervals(ctx, db.ListMentorBusyIntervalsParams{
		MentorID:    mentorID,
		WindowStart: pgTimestamptz(now.Add(-24 * time.Hour)),
		WindowEnd:   pgTimestamptz(windowEnd.Add(24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("ListMentorBusyIntervals after re-sync: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d stored intervals after a re-sync of the same period, want 1 (updated, not duplicated): %+v", len(rows), rows)
	}
}
