//go:build integration

// Integration test for the candidate query against a real Postgres — the unit tests
// in runner_test.go cover the runner's logic with fakes; this covers the SQL itself:
// that a free-tier account with a recent hit is found, a paying account is not, an
// old hit falls out of the window, and a send really does leave the row
// re-selectable. Run with: go test -tags=integration ./internal/engage/limitnudge/
package limitnudge

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

// makeVerifiedUser inserts a verified, free-tier account.
func makeVerifiedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, email_verified) VALUES ($1, true) RETURNING id`, email,
	).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// recordHit inserts a plan_limit_hits row `daysAgo` days back, exactly as
// plan.Store.Consume's recordLimitHit does on a real refusal.
func recordHit(t *testing.T, q *db.Queries, userID int64, daysAgo int) {
	t.Helper()
	day := time.Now().UTC().AddDate(0, 0, -daysAgo)
	if err := q.RecordPlanLimitHit(context.Background(), db.RecordPlanLimitHitParams{
		UserID: userID, Feature: "tailor",
		Day: pgtype.Date{Time: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), Valid: true},
	}); err != nil {
		t.Fatalf("record hit: %v", err)
	}
}

func TestRunnerAgainstPostgres_NudgesOnceAcrossTwoRuns(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	userID := makeVerifiedUser(t, pool, "nudge-once@example.test")
	recordHit(t, q, userID, 0)

	mailer := &fakeMailer{}
	r := New(q, mailer, 3, 500)

	first, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Sent != 1 || len(mailer.sent) != 1 || mailer.sent[0] != userID {
		t.Fatalf("first run = %+v, sent = %v, want one send to %d", first, mailer.sent, userID)
	}

	second, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Sent != 0 {
		t.Errorf("second run sent = %d, want 0 — the account was already nudged", second.Sent)
	}
	if len(mailer.sent) != 1 {
		t.Errorf("mailer.sent after the second run = %v, want still just the one call", mailer.sent)
	}
}

func TestRunnerAgainstPostgres_FailedSendStaysACandidate(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	userID := makeVerifiedUser(t, pool, "nudge-retry@example.test")
	recordHit(t, q, userID, 0)

	failing := &fakeMailer{failFor: userID}
	if _, err := New(q, failing, 3, 500).Run(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}

	working := &fakeMailer{}
	second, err := New(q, working, 3, 500).Run(context.Background())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Sent != 1 || len(working.sent) != 1 || working.sent[0] != userID {
		t.Fatalf("second run = %+v, sent = %v, want the retried account nudged", second, working.sent)
	}
}

func TestRunnerAgainstPostgres_PayingAccountIsNeverACandidate(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := db.New(pool)
	userID := makeVerifiedUser(t, pool, "paying-no-nudge@example.test")
	recordHit(t, q, userID, 0)
	if err := q.SetProUntilGranted(ctx, db.SetProUntilGrantedParams{
		ID: userID, Until: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("grant pro: %v", err)
	}

	mailer := &fakeMailer{}
	stats, err := New(q, mailer, 3, 500).Run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Sent != 0 || len(mailer.sent) != 0 {
		t.Errorf("stats = %+v, sent = %v — a paying account must never be nudged, even one with a recorded hit", stats, mailer.sent)
	}
}

func TestRunnerAgainstPostgres_HitOutsideTheWindowIsNotACandidate(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := db.New(pool)
	userID := makeVerifiedUser(t, pool, "stale-hit@example.test")
	recordHit(t, q, userID, 10) // outside the 3-day window below

	mailer := &fakeMailer{}
	stats, err := New(q, mailer, 3, 500).Run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Sent != 0 || len(mailer.sent) != 0 {
		t.Errorf("stats = %+v, sent = %v — a hit outside the window must not surface", stats, mailer.sent)
	}
}
