//go:build integration

// Integration tests for the per-company hiring-signal rollup (insights_company_stats).
// Like the other insights_* rollups the recompute lives entirely in SQL, so it is only
// verifiable against a real Postgres: seed jobs with known open/close ages, run the
// rebuild, and assert the per-(company, day) added/removed and running open counts.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// companyStatRow mirrors one insights_company_stats row for assertions.
type companyStatRow struct {
	day     string
	added   int
	removed int
	open    int
}

// seedCompanyJob inserts one job for a company with explicit open/close ages (days
// before now) and an optional duplicate_of parent, then returns its id. It writes the
// jobs table directly so the test controls company_slug, created_at, closed_at, and
// duplicate_of precisely.
func seedCompanyJob(t *testing.T, pool *pgxpool.Pool, ext, slug string, createdAgo int, closedAgo *int, dupOf *int64) int64 {
	t.Helper()
	var closed any
	if closedAgo != nil {
		closed = *closedAgo
	}
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug, duplicate_of,
			created_at, closed_at)
		VALUES ('test', $1, 'http://example.test', 'A job', 'job-' || $1, $2, $3,
			now() - make_interval(days => $4::int),
			CASE WHEN $5::int IS NULL THEN NULL ELSE now() - make_interval(days => $5::int) END)
		RETURNING id`,
		ext, slug, dupOf, createdAgo, closed).Scan(&id)
	if err != nil {
		t.Fatalf("seed company job %s: %v", ext, err)
	}
	return id
}

func companyStats(t *testing.T, ctx context.Context, pool *pgxpool.Pool, slug string) []companyStatRow {
	t.Helper()
	rows, err := pool.Query(ctx,
		`SELECT day::text, added, removed, open FROM insights_company_stats
		 WHERE company_slug = $1 ORDER BY day`, slug)
	if err != nil {
		t.Fatalf("query company stats: %v", err)
	}
	defer rows.Close()
	var out []companyStatRow
	for rows.Next() {
		var r companyStatRow
		if err := rows.Scan(&r.day, &r.added, &r.removed, &r.open); err != nil {
			t.Fatalf("scan company stat: %v", err)
		}
		out = append(out, r)
	}
	return out
}

func TestInsightsCompanyRollupVelocityAndRunningOpen(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// acme: two jobs opened 20d ago (one later closed 5d ago), a third opened 3d ago,
	// plus a repost copy (duplicate_of) that must be ignored.
	closed5 := 5
	a1 := seedCompanyJob(t, pool, "a1", "acme", 20, nil, nil)
	seedCompanyJob(t, pool, "a2", "acme", 20, &closed5, nil)
	seedCompanyJob(t, pool, "a3", "acme", 3, nil, nil)
	seedCompanyJob(t, pool, "adup", "acme", 10, nil, &a1) // duplicate → excluded

	// globex: one job, still open.
	seedCompanyJob(t, pool, "g1", "globex", 15, nil, nil)

	// company-less job → excluded entirely.
	seedCompanyJob(t, pool, "e1", "", 8, nil, nil)

	if err := q.DeleteAllInsightsCompanyStats(ctx); err != nil {
		t.Fatalf("delete company stats: %v", err)
	}
	if _, err := q.RebuildInsightsCompanyStats(ctx); err != nil {
		t.Fatalf("rebuild company stats: %v", err)
	}

	acme := companyStats(t, ctx, pool, "acme")
	var addedSum, removedSum int
	for _, r := range acme {
		addedSum += r.added
		removedSum += r.removed
	}
	// Three canonical jobs opened (a1,a2,a3); the duplicate contributes nothing.
	if addedSum != 3 {
		t.Errorf("acme added total = %d, want 3 (duplicate excluded)", addedSum)
	}
	// One job closed (a2).
	if removedSum != 1 {
		t.Errorf("acme removed total = %d, want 1", removedSum)
	}
	// The close day carries removed=1 and a running open of 1 (a1 still open, a3 not
	// yet opened as of 5d ago).
	var closeRow *companyStatRow
	for i := range acme {
		if acme[i].removed > 0 {
			closeRow = &acme[i]
		}
	}
	if closeRow == nil || closeRow.removed != 1 || closeRow.open != 1 {
		t.Errorf("acme close-day row = %+v, want removed=1 open=1", closeRow)
	}
	// The latest row's running open is the current open count: a1 + a3 = 2.
	if len(acme) == 0 || acme[len(acme)-1].open != 2 {
		t.Errorf("acme latest open = %v, want 2", acme)
	}

	// globex: one add, no removals, running open 1.
	glob := companyStats(t, ctx, pool, "globex")
	if len(glob) != 1 || glob[0].added != 1 || glob[0].removed != 0 || glob[0].open != 1 {
		t.Errorf("globex rows = %+v, want single {added 1, removed 0, open 1}", glob)
	}

	// company-less job produced no rows under the empty slug.
	if empty := companyStats(t, ctx, pool, ""); len(empty) != 0 {
		t.Errorf("empty-slug rows = %+v, want none", empty)
	}
}
