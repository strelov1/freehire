//go:build integration

// Integration tests for the close a retirement performs: a retired board is never crawled
// again, and Retire deletes its board_health row, which is the only evidence the chronic-board
// safety net closes from — so nothing else will ever close what the board left open. aijobs
// (freehire#3796) sat with 2,569 open postings for eight weeks that way.
// Run with: go test -tags=integration ./internal/ingest/boardcatalog/
package boardcatalog

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// insertOpenJob seeds an open posting under source/external_id, the two columns a
// retirement's close is scoped by.
func insertOpenJob(t *testing.T, pool *pgxpool.Pool, source, externalID string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		 VALUES ($1, $2, 'https://example.test', 'Engineer', $1 || '-' || $2, 'acme') RETURNING id`,
		source, externalID).Scan(&id); err != nil {
		t.Fatalf("insert job %s/%s: %v", source, externalID, err)
	}
	return id
}

// seedBoard writes a boards row directly, for the shapes the Inserter does not take.
func seedBoard(t *testing.T, pool *pgxpool.Pool, provider, board, region string, status Status) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO boards (provider, board, region, company, status, surface)
		 VALUES ($1, $2, $3, 'Acme', $4, 'curator')`, provider, board, region, string(status)); err != nil {
		t.Fatalf("seed board %s/%s/%s: %v", provider, board, region, err)
	}
}

// closedReason reports a job's closed_reason, or "open" while closed_at is NULL.
func closedReason(t *testing.T, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var reason string
	if err := pool.QueryRow(context.Background(),
		`SELECT CASE WHEN closed_at IS NULL THEN 'open' ELSE closed_reason END FROM jobs WHERE id = $1`,
		id).Scan(&reason); err != nil {
		t.Fatalf("read job %d: %v", id, err)
	}
	return reason
}

func TestRetireClosesTheBoardsOpenPostings(t *testing.T) {
	pool := testdb.Pool(t)
	repo := NewQueriesRepository(db.New(pool))
	ctx := context.Background()
	ins := NewInserter(repo, sources.Taxonomy())
	for _, board := range []string{"Gone_Co", "GoneXCo"} {
		if _, err := ins.Insert(ctx, InsertInput{Provider: "lever", Board: board, Company: board}, StatusActive); err != nil {
			t.Fatalf("Insert %s: %v", board, err)
		}
	}
	own := insertOpenJob(t, pool, "lever", "Gone_Co:1")
	// The underscore is LIKE syntax: unescaped, "Gone_Co:%" would claim this sibling too.
	sibling := insertOpenJob(t, pool, "lever", "GoneXCo:1")

	// Typed in a different case than the catalogue stores it, as a curator might: the
	// close must still scope by the stored spelling the postings were namespaced with.
	got, err := repo.Retire(ctx, "lever", "gone_co", "")
	if err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if !got.Retired || got.Closed != 1 {
		t.Fatalf("Retire = %+v, want retired with 1 posting closed", got)
	}
	if r := closedReason(t, pool, own); r != "board_retired" {
		t.Errorf("retired board's posting = %q, want closed as board_retired", r)
	}
	if r := closedReason(t, pool, sibling); r != "open" {
		t.Errorf("sibling board's posting = %q, want open", r)
	}
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM search_delete_outbox WHERE job_id = $1`, own).Scan(&queued); err != nil {
		t.Fatalf("count search_delete_outbox: %v", err)
	}
	if queued != 1 {
		t.Errorf("search_delete_outbox rows for the closed posting = %d, want 1", queued)
	}
}

// A boardless provider's postings carry the ":<id>" namespace, which is exactly what its
// empty board's pattern matches.
func TestRetireClosesABoardlessProvidersPostings(t *testing.T) {
	pool := testdb.Pool(t)
	repo := NewQueriesRepository(db.New(pool))
	ctx := context.Background()
	seedBoard(t, pool, "aijobs", "", "", StatusActive)
	own := insertOpenJob(t, pool, "aijobs", ":238344")
	other := insertOpenJob(t, pool, "greenhouse", ":238344")

	got, err := repo.Retire(ctx, "aijobs", "", "")
	if err != nil || !got.Retired || got.Closed != 1 {
		t.Fatalf("Retire = %+v,%v, want retired with 1 posting closed", got, err)
	}
	if r := closedReason(t, pool, own); r != "board_retired" {
		t.Errorf("provider's posting = %q, want closed as board_retired", r)
	}
	if r := closedReason(t, pool, other); r != "open" {
		t.Errorf("another provider's posting = %q, want open", r)
	}
}

// external_id carries no region, so while the same board name is still live in another
// region the retired one's postings cannot be told from the live one's. Closing would take
// the live region's postings down with it; the close refuses instead.
func TestRetireLeavesPostingsOpenWhileAnotherRegionOfTheBoardIsLive(t *testing.T) {
	pool := testdb.Pool(t)
	repo := NewQueriesRepository(db.New(pool))
	ctx := context.Background()
	seedBoard(t, pool, "adzuna", "it-jobs", "us", StatusActive)
	seedBoard(t, pool, "adzuna", "it-jobs", "gb", StatusActive)
	job := insertOpenJob(t, pool, "adzuna", "it-jobs:1")

	got, err := repo.Retire(ctx, "adzuna", "it-jobs", "gb")
	if err != nil || !got.Retired {
		t.Fatalf("Retire = %+v,%v, want retired", got, err)
	}
	if got.Closed != 0 {
		t.Errorf("Retire closed %d postings while the us region is live, want 0", got.Closed)
	}
	if r := closedReason(t, pool, job); r != "open" {
		t.Errorf("posting = %q, want open", r)
	}
}

// A board retired before Retire closed anything (aijobs, 2026-09-07) left its postings open.
// Retiring it again finds no live row to retire, and closes what it left behind.
func TestRetireOfAnAlreadyRetiredBoardClosesWhatItLeftOpen(t *testing.T) {
	pool := testdb.Pool(t)
	repo := NewQueriesRepository(db.New(pool))
	ctx := context.Background()
	seedBoard(t, pool, "aijobs", "", "", StatusRetired)
	job := insertOpenJob(t, pool, "aijobs", ":1")

	got, err := repo.Retire(ctx, "aijobs", "", "")
	if err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if got.Retired || got.Closed != 1 {
		t.Fatalf("Retire = %+v, want not newly retired, 1 posting closed", got)
	}
	if r := closedReason(t, pool, job); r != "board_retired" {
		t.Errorf("posting = %q, want closed as board_retired", r)
	}

	again, err := repo.Retire(ctx, "aijobs", "", "")
	if err != nil || again.Retired || again.Closed != 0 {
		t.Fatalf("second Retire = %+v,%v, want nothing retired or closed", again, err)
	}
}

// A board that never existed retires nothing and closes nothing — the not-found answer
// cmd/add-board reports.
func TestRetireOfAnUnknownBoardFindsNothing(t *testing.T) {
	repo := newRepo(t)
	got, err := repo.Retire(context.Background(), "greenhouse", "no-such-board", "")
	if err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if got != (Retirement{}) {
		t.Errorf("Retire = %+v, want nothing found", got)
	}
}
