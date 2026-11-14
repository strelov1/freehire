//go:build integration

package migrate_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/migrate"
	"github.com/strelov1/freehire/internal/platform/modroot"
)

func activityContractMigration(t *testing.T) migrate.Migration {
	t.Helper()
	root, err := modroot.Find()
	if err != nil {
		t.Fatal(err)
	}
	all, err := migrate.Load(filepath.Join(root, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Version == "0265_job_activity_contract.sql" {
			m.Version = "9999_activity_contract_test.sql"
			return m
		}
	}
	t.Fatal("activity contract migration missing")
	return migrate.Migration{}
}

func TestActivityContractPreservesObservationsAndRecordsOnce(t *testing.T) {
	pool := legacyActivityPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug,
		last_seen_at, hydrated_at, liveness_strikes)
		VALUES ('test', 'contract', 'https://example.test/contract', 'A', 'contract', '2020-01-01', NULL, 1);
		UPDATE job_activity SET last_seen_at = '2026-01-02T00:00:00Z', hydrated_at = '2026-01-01T00:00:00Z',
		liveness_strikes = 3, state_version = 7`); err != nil {
		t.Fatal(err)
	}
	var physicalBefore string
	if err := pool.QueryRow(ctx, `SELECT row(xmin::text, xmax::text, ctid::text)::text FROM jobs`).Scan(&physicalBefore); err != nil {
		t.Fatal(err)
	}
	m := activityContractMigration(t)
	runner := migrate.NewRunner(pool)
	if _, applied, err := runner.Run(ctx, []migrate.Migration{m}, false); err != nil || len(applied) != 1 {
		t.Fatalf("apply contract: applied=%v err=%v", applied, err)
	}
	var seen, hydrated time.Time
	var strikes int32
	var version int64
	if err := pool.QueryRow(ctx, `SELECT last_seen_at, hydrated_at, liveness_strikes, state_version
		FROM job_activity`).Scan(&seen, &hydrated, &strikes, &version); err != nil {
		t.Fatal(err)
	}
	if !seen.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) ||
		!hydrated.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || strikes != 3 || version != 7 {
		t.Fatalf("activity changed: %v %v strikes=%d version=%d", seen, hydrated, strikes, version)
	}
	var physicalAfter string
	if err := pool.QueryRow(ctx, `SELECT row(xmin::text, xmax::text, ctid::text)::text FROM jobs`).Scan(&physicalAfter); err != nil {
		t.Fatal(err)
	}
	if physicalBefore != physicalAfter {
		t.Fatalf("contract rewrote jobs: %s -> %s", physicalBefore, physicalAfter)
	}
	if _, applied, err := runner.Run(ctx, []migrate.Migration{m}, false); err != nil || len(applied) != 0 {
		t.Fatalf("recorded contract replayed: applied=%v err=%v", applied, err)
	}
}

func TestActivityContractRefusesUnvalidatedForeignKeyWithoutPartialCleanup(t *testing.T) {
	pool := legacyActivityPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `ALTER TABLE job_activity DROP CONSTRAINT job_activity_job_id_fkey;
		ALTER TABLE job_activity ADD CONSTRAINT job_activity_job_id_fkey
		FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE NOT VALID`); err != nil {
		t.Fatal(err)
	}
	m := activityContractMigration(t)
	if _, _, err := migrate.NewRunner(pool).Run(ctx, []migrate.Migration{m}, false); err == nil {
		t.Fatal("contract accepted an unvalidated foreign key")
	}
	if recorded(t, pool, m.Version) {
		t.Fatal("failed contract was recorded")
	}
	var columns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_attribute WHERE attrelid = 'jobs'::regclass
		AND NOT attisdropped AND attname IN ('last_seen_at', 'hydrated_at', 'liveness_strikes')`).Scan(&columns); err != nil || columns != 3 {
		t.Fatalf("failed contract partially removed legacy columns: count=%d err=%v", columns, err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE job_activity VALIDATE CONSTRAINT job_activity_job_id_fkey`); err != nil {
		t.Fatal(err)
	}
	if _, applied, err := migrate.NewRunner(pool).Run(ctx, []migrate.Migration{m}, false); err != nil || len(applied) != 1 {
		t.Fatalf("validated retry: applied=%v err=%v", applied, err)
	}
}
