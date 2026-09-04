//go:build integration

// Integration tests for the link_contributions carry, against a real Postgres: what
// destinationOf decides actually lands there, original timestamps survive, and a second
// run writes nothing. Where each status GOES is decided without a database — see
// main_test.go.
// Run with: go test -tags=integration ./cmd/backfill-link-contributions/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// carryAll runs the worker's loop over every row, reporting how many statements changed a
// row and how many found their destination already holding it.
func carryAll(t *testing.T, q *db.Queries, rows []db.ListLinkContributionsForBackfillRow) (wrote, already int) {
	t.Helper()
	ins := boardcatalog.NewInserter(boardcatalog.NewQueriesRepository(q), sources.Taxonomy())
	for _, r := range rows {
		dest, err := destinationOf(r)
		if err != nil {
			t.Fatalf("destinationOf(%s): %v", r.Status, err)
		}
		if !dest.writes() {
			continue
		}
		changed, err := write(context.Background(), q, ins, r, dest)
		if err != nil {
			t.Fatalf("write %s: %v", r.Status, err)
		}
		if changed {
			wrote++
		} else {
			already++
		}
	}
	return wrote, already
}

func insertUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ('carry@example.com') RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// seed writes one contribution of each status, plus the catalog row an 'onboarded'
// contribution is expected to find already there (carrying no submitter, exactly as the
// YAML backfill left it).
func seed(t *testing.T, pool *pgxpool.Pool, user int64, when time.Time) {
	t.Helper()
	ctx := context.Background()
	rows := []struct{ url, source, board, status string }{
		{"https://example.com/review-link", "", "", "review"},
		{"https://example.com/pending-link", "ashby", "onsign", "pending"},
		{"https://example.com/onboarded-link", "greenhouse", "acme", "onboarded"},
		{"https://example.com/rejected-link", "lever", "deadco", "rejected"},
	}
	for _, r := range rows {
		var source, board any
		if r.source != "" {
			source, board = r.source, r.board
		}
		_, err := pool.Exec(ctx,
			`INSERT INTO link_contributions (submitted_by, url, source, board, status, surface, created_at)
			 VALUES ($1, $2, $3, $4, $5, 'web', $6)`,
			user, r.url, source, board, r.status, when)
		if err != nil {
			t.Fatalf("seed %s: %v", r.status, err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO boards (provider, board, region, company, status)
		 VALUES ('greenhouse', 'acme', '', 'Acme', 'active')`); err != nil {
		t.Fatalf("seed catalog row: %v", err)
	}
}

func TestCarryPlacesEveryStatusAndIsIdempotent(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	user := insertUser(t, pool)
	when := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	seed(t, pool, user, when)

	rows, err := q.ListLinkContributionsForBackfill(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("listed %d contributions, want 4", len(rows))
	}

	if wrote, already := carryAll(t, q, rows); wrote != 3 || already != 0 {
		t.Fatalf("first run wrote %d rows (%d already there), want 3 and 0", wrote, already)
	}

	// The unclassified URL is queued for triage, under its ORIGINAL date: restamping it
	// as today would reorder every user's contributions list.
	var subCreated time.Time
	if err := pool.QueryRow(ctx,
		`SELECT created_at FROM board_submissions WHERE url = 'https://example.com/review-link'`,
	).Scan(&subCreated); err != nil {
		t.Fatalf("the review row must reach board_submissions: %v", err)
	}
	if !subCreated.UTC().Equal(when) {
		t.Errorf("submission created_at = %s, want the original %s", subCreated.UTC(), when)
	}

	// The recognized-but-unonboarded board is crawlable again.
	assertBoard(t, pool, "ashby", "onsign", "pending", user)
	// The refusal is deliberately NOT carried: 0049 already freed the identity, so the
	// board can be re-contributed and judged again on the day.
	assertNoBoard(t, pool, "lever", "deadco")
	// The already-onboarded board keeps its single row and gains its submitter.
	assertBoard(t, pool, "greenhouse", "acme", "active", user)

	// A second run is inert: every statement is conflict- or NULL-guarded, so stopping
	// the worker mid-way and re-running it costs nothing.
	if wrote, already := carryAll(t, q, rows); wrote != 0 || already != 3 {
		t.Errorf("second run wrote %d rows (%d already there), want 0 and 3", wrote, already)
	}

	var boards int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM boards`).Scan(&boards); err != nil {
		t.Fatalf("count boards: %v", err)
	}
	if boards != 2 {
		t.Errorf("boards = %d, want 2 (the seeded one plus the carried pending) — "+
			"the re-run must not add another", boards)
	}
}

// assertNoBoard is the counterpart: the identity must be free, so a later contribution of
// the same board is accepted rather than colliding with a carried-over refusal.
func assertNoBoard(t *testing.T, pool *pgxpool.Pool, provider, board string) {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM boards WHERE provider = $1 AND board = $2`,
		provider, board).Scan(&n); err != nil {
		t.Fatalf("count %s/%s: %v", provider, board, err)
	}
	if n != 0 {
		t.Errorf("%s/%s is in the catalog (%d rows); a refusal must not be carried", provider, board, n)
	}
}

func assertBoard(t *testing.T, pool *pgxpool.Pool, provider, board, wantStatus string, wantUser int64) {
	t.Helper()
	var status string
	var submitter *int64
	err := pool.QueryRow(context.Background(),
		`SELECT status, submitted_by FROM boards WHERE provider = $1 AND board = $2`,
		provider, board).Scan(&status, &submitter)
	if err != nil {
		t.Fatalf("%s/%s not in the catalog: %v", provider, board, err)
	}
	if status != wantStatus {
		t.Errorf("%s/%s status = %q, want %q", provider, board, status, wantStatus)
	}
	if submitter == nil || *submitter != wantUser {
		t.Errorf("%s/%s submitter = %v, want %d — the contribution must stay attributed",
			provider, board, submitter, wantUser)
	}
}

// A recognized status with no (source, board) names nothing that can be carried. It is
// counted rather than dropped: a nonzero skip means the schema's own assumption — only
// 'review' carries a NULL source — no longer holds, and the row needs a human.
func TestCarrySkipsARecognizedRowWithNoBoard(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	user := insertUser(t, pool)

	if _, err := pool.Exec(ctx,
		`INSERT INTO link_contributions (submitted_by, url, source, board, status, surface)
		 VALUES ($1, 'https://example.com/x', NULL, NULL, 'pending', 'web')`, user); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rows, err := q.ListLinkContributionsForBackfill(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("listed %d rows, want 1", len(rows))
	}
	dest, err := destinationOf(rows[0])
	if err != nil {
		t.Fatalf("destinationOf: %v", err)
	}
	if dest != unplaceable {
		t.Errorf("destinationOf = %q, want %q", dest, unplaceable)
	}
}

// An 'onboarded' contribution whose board is NOT in the catalog is the case that decides
// whether the drop is safe. The UPDATE changes nothing, exactly as it does for a row
// already attributed — but the two mean opposite things: one is done, the other names a
// board nothing crawls. Measured on prod: 163 match, 9 do not.
//
// Telling them apart needs a second question, and getting it wrong would let the drop take
// those 9 while the run reported them as already carried.
func TestAnOnboardedContributionWithNoCatalogRowIsNotAlreadyCarried(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	repo := boardcatalog.NewQueriesRepository(q)
	user := insertUser(t, pool)

	if _, err := pool.Exec(ctx,
		`INSERT INTO link_contributions (submitted_by, url, source, board, status, surface)
		 VALUES ($1, 'https://example.com/orphan', 'greenhouse', 'nowhere', 'onboarded', 'web')`,
		user); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rows, err := q.ListLinkContributionsForBackfill(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	wrote, already := carryAll(t, q, rows)
	if wrote != 0 || already != 1 {
		t.Fatalf("carry wrote %d (%d unchanged), want 0 and 1", wrote, already)
	}
	// The unchanged UPDATE is ambiguous; this is the question that resolves it.
	listed, err := inCatalog(ctx, repo, rows[0])
	if err != nil {
		t.Fatalf("inCatalog: %v", err)
	}
	if listed {
		t.Error("the catalog does not carry this board; the run must not read it as carried")
	}

	// And the ordinary case still answers the other way.
	if _, err := pool.Exec(ctx,
		`INSERT INTO boards (provider, board, region, company, status)
		 VALUES ('greenhouse', 'nowhere', '', 'Nowhere', 'active')`); err != nil {
		t.Fatalf("seed catalog row: %v", err)
	}
	listed, err = inCatalog(ctx, repo, rows[0])
	if err != nil {
		t.Fatalf("inCatalog: %v", err)
	}
	if !listed {
		t.Error("a board the catalog carries must read as listed")
	}
}
