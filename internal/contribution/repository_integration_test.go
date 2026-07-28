//go:build integration

// Integration tests for the contribution repository against a real Postgres: every
// submission of a board is recorded, and exactly one of them — even under a concurrent
// race — is reported as rewardable. The AI-credits reward itself is granted by the handler,
// not the repository, so it is not exercised here.
// Run with: go test -tags=integration ./internal/contribution/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package contribution

import (
	"context"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/testdb"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

func insertUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func insertJob(t *testing.T, pool *pgxpool.Pool, source, externalID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ($1, $2, 'http://example.test', 'A job', 'job-' || $2)`,
		source, externalID); err != nil {
		t.Fatalf("insert job: %v", err)
	}
}

// TestRecordKeepsEverySubmissionAndRewardsTheFirst pins the rule that replaced
// UNIQUE (source, board): a second link to a board is recorded (it is evidence the board is
// worth onboarding, and it names its own submitter) but earns nothing.
func TestRecordKeepsEverySubmissionAndRewardsTheFirst(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(pool, db.New(pool))
	userID := insertUser(t, pool, "u@example.test")

	in := RecordInput{SubmittedBy: userID, URL: "https://jobs.ashbyhq.com/blitzy", Source: "ashby", Board: "blitzy", Surface: SurfaceWeb}

	c, rewardable, err := repo.Record(ctx, in)
	if err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if c.ID == 0 || c.Status != "pending" || c.Board != "blitzy" {
		t.Errorf("recorded row unexpected: %+v", c)
	}
	if !rewardable {
		t.Error("first submission of a board is not rewardable, want rewardable")
	}
	if c.Surface != SurfaceWeb {
		t.Errorf("surface = %q, want %q", c.Surface, SurfaceWeb)
	}

	// Same board again via a different vacancy URL: recorded, but no second reward.
	dup := RecordInput{SubmittedBy: userID, URL: "https://jobs.ashbyhq.com/blitzy/another-uuid", Source: "ashby", Board: "blitzy", Surface: SurfaceCLI}
	second, rewardable, err := repo.Record(ctx, dup)
	if err != nil {
		t.Fatalf("second Record: %v", err)
	}
	if rewardable {
		t.Error("second submission of the same board is rewardable, want not rewardable")
	}
	if second.ID == c.ID {
		t.Error("second submission reused the first row, want its own row")
	}
	if n := countContributions(t, pool, userID); n != 2 {
		t.Errorf("contributions after a repeat = %d, want 2 — every submission is kept", n)
	}
}

func countContributions(t *testing.T, pool *pgxpool.Pool, userID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM link_contributions WHERE submitted_by = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count contributions: %v", err)
	}
	return n
}

func TestBoardTracked(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(pool, db.New(pool))

	insertJob(t, pool, "greenhouse", "acme:100")

	// Board tracked; a different board is not.
	if ok, err := repo.BoardTracked(ctx, "greenhouse", "acme"); err != nil || !ok {
		t.Errorf("BoardTracked(acme) = %v,%v, want true", ok, err)
	}
	if ok, err := repo.BoardTracked(ctx, "greenhouse", "globex"); err != nil || ok {
		t.Errorf("BoardTracked(globex) = %v,%v, want false", ok, err)
	}
	// A LIKE metacharacter in the board must not widen the match: "ac_e" must not match "acme".
	if ok, err := repo.BoardTracked(ctx, "greenhouse", "ac_e"); err != nil || ok {
		t.Errorf("BoardTracked(ac_e) = %v,%v, want false — '_' must be escaped, not a wildcard", ok, err)
	}
}

// TestRecordConcurrentRewardsExactlyOne is the test the dropped unique constraint used to
// make unnecessary. Two submissions of one new board racing must both be recorded, and
// exactly one may be rewarded — a plain EXISTS check cannot do this (both transactions read
// the same snapshot and both see no rows), so it proves the advisory lock is doing its job.
func TestRecordConcurrentRewardsExactlyOne(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(pool, db.New(pool))
	userID := insertUser(t, pool, "race@example.test")

	in := RecordInput{SubmittedBy: userID, URL: "https://jobs.lever.co/acme", Source: "lever", Board: "acme", Surface: SurfaceWeb}

	const racers = 4
	var wg sync.WaitGroup
	rewards := make([]bool, racers)
	errs := make([]error, racers)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, rewards[i], errs[i] = repo.Record(ctx, in)
		}(i)
	}
	wg.Wait()

	rewarded := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("racer %d: unexpected error: %v", i, err)
		}
		if rewards[i] {
			rewarded++
		}
	}
	if rewarded != 1 {
		t.Errorf("rewarded = %d, want exactly 1 — the board may be paid for once", rewarded)
	}
	if got := countContributions(t, pool, userID); got != racers {
		t.Errorf("contributions after race = %d, want %d — every submission is kept", got, racers)
	}
}
