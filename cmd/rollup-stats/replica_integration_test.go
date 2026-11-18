//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestRollupReadsAnalyticsAndPublishesToPrimary(t *testing.T) {
	ctx := context.Background()
	analytics, primary := testdb.Pool(t), testdb.Pool(t)
	if _, err := analytics.Exec(ctx, `INSERT INTO jobs (public_slug,source,external_id,title,url,company,company_slug,category,seniority,skills,countries,created_at) VALUES ('test-analytics','test','analytics','Engineer','https://example.com/job','Example','example','engineering','senior',ARRAY['go'],ARRAY['us'],now()-interval '60 days')`); err != nil {
		t.Fatal(err)
	}
	if _, err := primary.Exec(ctx, `INSERT INTO jobs (public_slug,source,external_id,title,url,company,company_slug,category,seniority,created_at) VALUES ('test-primary','test','primary','Other','https://example.com/other','Other','other','design','junior',now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := analytics.Exec(ctx, `ALTER DATABASE "`+analytics.Config().ConnConfig.Database+`" SET default_transaction_read_only = on`); err != nil {
		t.Fatal(err)
	}
	analytics.Reset()
	if err := rebuildRollups(ctx, analytics, primary); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := primary.QueryRow(ctx, `SELECT open_count FROM insights_role_stats WHERE category='engineering' AND seniority='senior' AND country=''`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("analytics count=%d, want 1", count)
	}
	if err := primary.QueryRow(ctx, `SELECT count(*) FROM insights_role_stats WHERE category='design'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("primary catalogue leaked into rollup: %d rows", count)
	}
	if err := analytics.QueryRow(ctx, `SELECT count(*) FROM insights_role_stats`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("analytics database was written")
	}
}

func TestRollupReadFailureKeepsPublishedSnapshot(t *testing.T) {
	ctx := context.Background()
	analytics, primary := testdb.Pool(t), testdb.Pool(t)
	if _, err := primary.Exec(ctx, `INSERT INTO job_daily_stats(day,added,removed) VALUES ('2026-01-01',7,2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := analytics.Exec(ctx, `DROP VIEW jobs_effective_salary`); err != nil {
		t.Fatal(err)
	}
	if err := rebuildRollups(ctx, analytics, primary); err == nil {
		t.Fatal("missing analytics view must fail")
	}
	var added int
	if err := primary.QueryRow(ctx, `SELECT added FROM job_daily_stats WHERE day='2026-01-01'`).Scan(&added); err != nil {
		t.Fatal(err)
	}
	if added != 7 {
		t.Fatalf("published count=%d, want 7", added)
	}
}

func TestRollupPublicationFailureRollsBackAllTables(t *testing.T) {
	ctx := context.Background()
	analytics, primary := testdb.Pool(t), testdb.Pool(t)
	if _, err := analytics.Exec(ctx, `INSERT INTO jobs(public_slug,source,external_id,title,url,company,company_slug,category,seniority) VALUES ('test-rollback','test','rollback','Engineer','https://example.com/rollback','Example','example','engineering','senior')`); err != nil {
		t.Fatal(err)
	}
	if _, err := primary.Exec(ctx, `INSERT INTO job_daily_stats(day,added,removed) VALUES ('2026-01-01',7,2); ALTER TABLE insights_role_stats ADD CONSTRAINT injected_copy_failure CHECK (open_count < 0)`); err != nil {
		t.Fatal(err)
	}
	if err := rebuildRollups(ctx, analytics, primary); err == nil {
		t.Fatal("publication constraint failure must fail the rebuild")
	}
	var added int
	if err := primary.QueryRow(ctx, `SELECT added FROM job_daily_stats WHERE day='2026-01-01'`).Scan(&added); err != nil {
		t.Fatal(err)
	}
	if added != 7 {
		t.Fatalf("partial publication changed prior count to %d", added)
	}
}

func TestRollupDoesNotPinPrimaryVacuumWhileReadingAnalytics(t *testing.T) {
	analytics, primary := testdb.Pool(t), testdb.Pool(t)
	cfg := primary.Config()
	cfg.ConnConfig.RuntimeParams["application_name"] = "rollup-vacuum-test"
	writer, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := primary.Exec(ctx, `INSERT INTO job_daily_stats(day,added,removed) VALUES ('2026-01-01',7,2)`); err != nil {
		t.Fatal(err)
	}
	lock, err := analytics.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(context.Background())
	if _, err := lock.Exec(ctx, `LOCK TABLE jobs IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- rebuildRollups(ctx, analytics, writer) }()
	for {
		var blocked bool
		if err := analytics.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock')`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("rollup ended before blocked analytics read: %v", err)
		case <-ctx.Done():
			t.Fatal("analytics query did not reach the lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	var pinned int
	if err := primary.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name='rollup-vacuum-test' AND (backend_xid IS NOT NULL OR backend_xmin IS NOT NULL)`).Scan(&pinned); err != nil {
		t.Fatal(err)
	}
	if pinned != 0 {
		t.Fatalf("analytics read pinned %d primary transaction(s)", pinned)
	}
	if err := lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
