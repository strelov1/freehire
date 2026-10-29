//go:build integration

// Integration test for the per-source quality counts AggregateOpenJobsBySource carries
// beside open_jobs. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestAggregateOpenJobsBySourceCountsQualityGaps(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE companies, jobs RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Each row is open and fails exactly the checks its comment names, so a count that
	// read the wrong predicate lands on the wrong number rather than coinciding.
	seed := []string{
		// complete: counted in open_jobs and in no check.
		`('src', 'ok', 'acme', 'remote', '{de}', '{}', now(), now())`,
		// no work mode: the column is NOT NULL and an unknown mode is the empty string.
		`('src', 'nomode', 'acme', '', '{de}', '{}', now(), now())`,
		// no geography; regions alone would have counted as geography.
		`('src', 'nogeo', 'acme', 'onsite', '{}', '{}', now(), now())`,
		`('src', 'regiononly', 'acme', 'onsite', '{}', '{eu}', now(), now())`,
		// no company: company_slug is NOT NULL and an unknown employer is the empty string.
		`('src', 'nocompany', '', 'remote', '{de}', '{}', now(), now())`,
		// unseen by its crawl for longer than a week.
		`('src', 'unseen', 'acme', 'remote', '{de}', '{}', now() - interval '8 days', now())`,
		// posted more than 60 days ago.
		`('src', 'old', 'acme', 'remote', '{de}', '{}', now(), now() - interval '61 days')`,
	}
	for _, values := range seed {
		if _, err := pool.Exec(ctx,
			`INSERT INTO jobs (source, external_id, company_slug, work_mode, countries, regions, last_seen_at, posted_at,
			                   url, title, public_slug)
			 SELECT s, e, c, w, co::text[], r::text[], l, p, 'http://example.test', 'A job', 'job-' || e
			 FROM (VALUES `+values+`) AS v(s, e, c, w, co, r, l, p)`); err != nil {
			t.Fatalf("seed %s: %v", values, err)
		}
	}
	// No posting date: its age falls back to when we first stored it.
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, company_slug, work_mode, countries, url, title, public_slug, created_at)
		 VALUES ('src', 'undated', 'acme', 'remote', '{de}', 'http://example.test', 'A job', 'job-undated',
		         now() - interval '61 days')`); err != nil {
		t.Fatalf("seed undated: %v", err)
	}
	// A closed posting with every gap is not part of the catalogue and counts nowhere.
	if _, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, public_slug, closed_at)
		 VALUES ('src', 'closed', 'http://example.test', 'A job', 'job-closed', now())`); err != nil {
		t.Fatalf("seed closed: %v", err)
	}

	rows, err := q.AggregateOpenJobsBySource(ctx)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d sources, want 1: %+v", len(rows), rows)
	}
	got := rows[0]
	want := AggregateOpenJobsBySourceRow{
		Source:              "src",
		OpenJobs:            8,
		MissingWorkModeJobs: 1,
		MissingGeoJobs:      1,
		MissingCompanyJobs:  1,
		Unseen7dJobs:        1,
		PostedOver60dJobs:   2,
	}
	if got != want {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// A row measured before migration 0213 carries NULL counts until the next rebuild, and must
// not be published: as zeros it would read as "nothing missing" for every source.
func TestSourceQualityMetricsSkipsUnmeasuredRows(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE source_stats"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO source_stats (source, open_jobs, ats_matched_jobs) VALUES ('premigration', 10, 0)`); err != nil {
		t.Fatalf("seed unmeasured row: %v", err)
	}
	if err := q.InsertSourceStat(ctx, InsertSourceStatParams{
		Source: "rebuilt", OpenJobs: 10, MeasuredAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		MissingGeoJobs: 4,
	}); err != nil {
		t.Fatalf("insert rebuilt row: %v", err)
	}

	rows, err := q.SourceQualityMetrics(ctx)
	if err != nil {
		t.Fatalf("source quality metrics: %v", err)
	}
	if len(rows) != 1 || rows[0].Source != "rebuilt" || rows[0].MissingGeoJobs != 4 || rows[0].MissingWorkModeJobs != 0 {
		t.Errorf("got %+v, want only the rebuilt row", rows)
	}
}
