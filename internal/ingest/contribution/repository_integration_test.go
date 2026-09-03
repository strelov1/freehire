//go:build integration

// Integration tests for the contribution repository against a real Postgres: a
// recognized board inserts into `boards` (status=pending, immediately crawl-eligible),
// an unrecognized link inserts into `board_submissions`, and both feed ListByUser. The
// uniqueness on a live (pending/active) identity rejects a duplicate board (mapped to
// ErrBoardAlreadyContributed) while a rejected or retired row releases it.
// Run with: go test -tags=integration ./internal/ingest/contribution/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package contribution

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
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

func TestRecordInsertsAPendingBoard(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool))
	userID := insertUser(t, pool, "u@example.test")

	in := RecordInput{SubmittedBy: userID, URL: "https://jobs.ashbyhq.com/blitzy", Source: "ashby", Board: "blitzy", Surface: SurfaceCLI}

	c, err := repo.Record(ctx, in)
	if err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if c.ID == 0 || c.Status != "pending" || c.Board != "blitzy" {
		t.Errorf("recorded row unexpected: %+v", c)
	}
	if c.Surface != SurfaceCLI {
		t.Errorf("surface = %q, want %q", c.Surface, SurfaceCLI)
	}

	// A recognized-but-network-free submission has no real company name; it is seeded
	// with a placeholder derived from the board id rather than left empty, since
	// board-based adapters write it verbatim as every crawled job's employer.
	var company string
	if err := pool.QueryRow(ctx, `SELECT company FROM boards WHERE id = $1`, c.ID).Scan(&company); err != nil {
		t.Fatalf("read company: %v", err)
	}
	if company == "" || company == "blitzy" {
		t.Errorf("company = %q, want a humanized placeholder, not empty or the raw board id", company)
	}

	// Same board again (e.g. via a different vacancy URL) → rejected, no second row.
	dup := RecordInput{SubmittedBy: userID, URL: "https://jobs.ashbyhq.com/blitzy/another-uuid", Source: "ashby", Board: "blitzy"}
	_, err = repo.Record(ctx, dup)
	if !errors.Is(err, ErrBoardAlreadyContributed) {
		t.Fatalf("second Record err = %v, want ErrBoardAlreadyContributed", err)
	}
	if n := countBoardsFor(t, pool, userID); n != 1 {
		t.Errorf("boards after duplicate = %d, want still 1 — rejected insert must not record", n)
	}
}

// A rejected board is not a claim on the identity: a board turned down as dead can come
// back (the employer starts posting again), and the flow's own documentation promises it
// can be re-contributed. The uniqueness that guards the crawl queue therefore covers the
// live statuses (pending/active) only.
func TestRecordReopensARejectedBoard(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool))
	first := insertUser(t, pool, "rejected-first@example.com")
	second := insertUser(t, pool, "rejected-second@example.com")

	in := RecordInput{SubmittedBy: first, URL: "https://jobs.ashbyhq.com/dormant", Source: "ashby", Board: "dormant"}
	rec, err := repo.Record(ctx, in)
	if err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE boards SET status = 'rejected' WHERE id = $1`, rec.ID); err != nil {
		t.Fatalf("reject the row: %v", err)
	}

	// The board is alive again and somebody else pastes it.
	again := RecordInput{SubmittedBy: second, URL: "https://jobs.ashbyhq.com/dormant/a-uuid", Source: "ashby", Board: "dormant"}
	reopened, err := repo.Record(ctx, again)
	if err != nil {
		t.Fatalf("re-contributing a rejected board: %v, want it recorded", err)
	}
	if reopened.Status != "pending" || reopened.ID == rec.ID {
		t.Errorf("reopened row = %+v, want a new pending row", reopened)
	}
	if n := countBoardsFor(t, pool, second); n != 1 {
		t.Errorf("boards for the second user = %d, want 1", n)
	}
}

// The live statuses still hold the identity: an already-active board is in the catalogue,
// so a second contribution of it adds nothing and must not be rewarded.
func TestRecordStillRejectsDuplicateOfAnActiveBoard(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool))
	userID := insertUser(t, pool, "active-dup@example.com")

	rec, err := repo.Record(ctx, RecordInput{
		SubmittedBy: userID, URL: "https://jobs.ashbyhq.com/live", Source: "ashby", Board: "live",
	})
	if err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE boards SET status = 'active' WHERE id = $1`, rec.ID); err != nil {
		t.Fatalf("activate the row: %v", err)
	}

	_, err = repo.Record(ctx, RecordInput{
		SubmittedBy: userID, URL: "https://jobs.ashbyhq.com/live/uuid", Source: "ashby", Board: "live",
	})
	if !errors.Is(err, ErrBoardAlreadyContributed) {
		t.Fatalf("duplicate of an active board err = %v, want ErrBoardAlreadyContributed", err)
	}
}

func countBoardsFor(t *testing.T, pool *pgxpool.Pool, userID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM boards WHERE submitted_by = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count boards: %v", err)
	}
	return n
}

func TestBoardTracked(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool))

	insertJob(t, pool, "greenhouse", "acme:100")

	if ok, err := repo.BoardTracked(ctx, "greenhouse", "acme"); err != nil || !ok {
		t.Errorf("BoardTracked(acme) = %v,%v, want true", ok, err)
	}
	if ok, err := repo.BoardTracked(ctx, "greenhouse", "globex"); err != nil || ok {
		t.Errorf("BoardTracked(globex) = %v,%v, want false", ok, err)
	}
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
	if got := countBoardsFor(t, pool, userID); got != 1 {
		t.Errorf("boards after race = %d, want exactly 1", got)
	}
}

func TestRecordReviewInsertsAndDedupsAndFeedsListByUser(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool))
	userID := insertUser(t, pool, "review@example.test")

	rec, err := repo.RecordReview(ctx, userID, "https://example.test/careers/1", SurfaceWeb)
	if err != nil {
		t.Fatalf("RecordReview: %v", err)
	}
	if rec.Status != StatusReview || rec.Source != "" || rec.Board != "" {
		t.Errorf("RecordReview result = %+v, want status=review, no source/board", rec)
	}

	// A duplicate URL is rejected the same way a duplicate board is.
	_, err = repo.RecordReview(ctx, userID, "https://example.test/careers/1", SurfaceWeb)
	if !errors.Is(err, ErrBoardAlreadyContributed) {
		t.Fatalf("duplicate RecordReview err = %v, want ErrBoardAlreadyContributed", err)
	}

	// It shows up in ListByUser alongside a recognized board.
	if _, err := repo.Record(ctx, RecordInput{SubmittedBy: userID, URL: "https://jobs.lever.co/acme2", Source: "lever", Board: "acme2"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	rows, err := repo.ListByUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListByUser = %+v, want 2 rows (one review, one board)", rows)
	}
	var sawReview, sawBoard bool
	for _, r := range rows {
		switch r.Status {
		case StatusReview:
			sawReview = true
			if r.Source != "" || r.Board != "" {
				t.Errorf("review row carries source/board: %+v", r)
			}
		case StatusPending:
			sawBoard = true
			if r.Board != "acme2" {
				t.Errorf("board row = %+v, want board=acme2", r)
			}
		}
	}
	if !sawReview || !sawBoard {
		t.Errorf("ListByUser = %+v, want one review row and one pending board row", rows)
	}
}

// ListByUser must never reveal another user's rows, in either table.
func TestListByUserIsScopedToTheCaller(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool))
	alice := insertUser(t, pool, "alice-list@example.test")
	bob := insertUser(t, pool, "bob-list@example.test")

	if _, err := repo.Record(ctx, RecordInput{SubmittedBy: alice, URL: "https://jobs.lever.co/a", Source: "lever", Board: "a"}); err != nil {
		t.Fatalf("Record alice: %v", err)
	}
	if _, err := repo.RecordReview(ctx, bob, "https://example.test/bob-only", SurfaceWeb); err != nil {
		t.Fatalf("RecordReview bob: %v", err)
	}

	rows, err := repo.ListByUser(ctx, alice)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(rows) != 1 || rows[0].Board != "a" {
		t.Fatalf("ListByUser(alice) = %+v, want only alice's board", rows)
	}
}
