//go:build integration

// Integration tests for the contribution repository against a real Postgres: the
// UNIQUE (source, board) constraint rejects a duplicate identity (mapped to
// ErrBoardAlreadyContributed) and — under a concurrent race — records exactly one board.
// The AI-credits reward is granted by the handler, not the repository, so it is not
// exercised here. Run with: go test -tags=integration ./internal/contribution/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package contribution

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/strelov1/freehire/internal/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	scripts, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("list migrations: %v (found %d)", err, len(scripts))
	}
	sort.Strings(scripts)

	pg, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("hire"),
		postgres.WithUsername("hire"),
		postgres.WithPassword("hire"),
		postgres.WithInitScripts(scripts...),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
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

func TestRecordAndDedups(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool))
	userID := insertUser(t, pool, "u@example.test")

	in := RecordInput{SubmittedBy: userID, URL: "https://jobs.ashbyhq.com/blitzy", Source: "ashby", Board: "blitzy"}

	c, err := repo.Record(ctx, in)
	if err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if c.ID == 0 || c.Status != "pending" || c.Board != "blitzy" {
		t.Errorf("recorded row unexpected: %+v", c)
	}

	// Same board again (e.g. via a different vacancy URL) → rejected, no second row.
	dup := RecordInput{SubmittedBy: userID, URL: "https://jobs.ashbyhq.com/blitzy/another-uuid", Source: "ashby", Board: "blitzy"}
	_, err = repo.Record(ctx, dup)
	if !errors.Is(err, ErrBoardAlreadyContributed) {
		t.Fatalf("second Record err = %v, want ErrBoardAlreadyContributed", err)
	}
	if n := countContributions(t, pool, userID); n != 1 {
		t.Errorf("contributions after duplicate = %d, want still 1 — rejected insert must not record", n)
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
	repo := NewQueriesRepository(db.New(pool))

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

func TestRecordConcurrentDuplicateRecordsOnce(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool))
	userID := insertUser(t, pool, "race@example.test")

	in := RecordInput{SubmittedBy: userID, URL: "https://jobs.lever.co/acme", Source: "lever", Board: "acme"}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = repo.Record(ctx, in)
		}(i)
	}
	wg.Wait()

	var ok, dup int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrBoardAlreadyContributed):
			dup++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || dup != 1 {
		t.Errorf("race outcome ok=%d dup=%d, want 1 and 1", ok, dup)
	}
	if got := countContributions(t, pool, userID); got != 1 {
		t.Errorf("contributions after race = %d, want exactly 1", got)
	}
}
