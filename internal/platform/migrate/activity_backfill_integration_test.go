//go:build integration

package migrate_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/migrate"
	"github.com/strelov1/freehire/internal/platform/modroot"
)

func TestActivityBackfillCommitsRangesAndPreservesMirroredRows(t *testing.T) {
	pool := bootstrapped(t)
	ctx := context.Background()
	// Model an existing catalogue at the beginning of release A. Gaps in IDs
	// exercise separate copy transactions without thousands of fixture rows.
	_, err := pool.Exec(ctx, `
		ALTER TABLE job_activity DROP CONSTRAINT job_activity_job_id_fkey;
		ALTER TABLE jobs DISABLE TRIGGER job_activity_insert;
		INSERT INTO jobs (id, source, external_id, url, title, public_slug, last_seen_at)
		OVERRIDING SYSTEM VALUE
		VALUES (1, 'test', 'a', 'https://example.test/a', 'A', 'backfill-a', '2020-01-01'),
		       (10001, 'test', 'b', 'https://example.test/b', 'B', 'backfill-b', '2020-01-01'),
		       (20001, 'test', 'c', 'https://example.test/c', 'C', 'backfill-c', '2020-01-01');
		ALTER TABLE jobs ENABLE TRIGGER job_activity_insert;
		UPDATE jobs SET last_seen_at='2021-01-01' WHERE id=20001;
		INSERT INTO job_activity (job_id) VALUES (99999);
	`)
	if err != nil {
		t.Fatal(err)
	}
	var mirroredXmin string
	var parentTuplesBefore string
	if err := pool.QueryRow(ctx, `SELECT json_agg(ROW(id,xmin::text,xmax::text,ctid::text) ORDER BY id)::text FROM jobs`).Scan(&parentTuplesBefore); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT xmin::text FROM job_activity WHERE job_id=20001`).Scan(&mirroredXmin); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"9998_", "9999_"} {
		if _, _, err := migrate.NewRunner(pool).Run(ctx, activityBackfillMigrations(t, prefix), false); err != nil {
			t.Fatal(err)
		}
	}
	var rows, transactions, orphans int
	var parentTuplesAfter string
	var copiedMirroredXmin string
	var validated bool
	if err := pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT xmin::text) FROM job_activity`).Scan(&rows, &transactions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_activity WHERE job_id=99999`).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT xmin::text FROM job_activity WHERE job_id=20001`).Scan(&copiedMirroredXmin); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT json_agg(ROW(id,xmin::text,xmax::text,ctid::text) ORDER BY id)::text FROM jobs`).Scan(&parentTuplesAfter); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT convalidated FROM pg_constraint WHERE conrelid='job_activity'::regclass AND conname='job_activity_job_id_fkey'`).Scan(&validated); err != nil {
		t.Fatal(err)
	}
	if rows != 3 || transactions < 3 || orphans != 0 || parentTuplesBefore != parentTuplesAfter || !validated || mirroredXmin != copiedMirroredXmin {
		t.Fatalf("rows=%d transactions=%d orphans=%d parent tuples unchanged=%v validated=%v mirrored tuple unchanged=%v", rows, transactions, orphans, parentTuplesBefore == parentTuplesAfter, validated, mirroredXmin == copiedMirroredXmin)
	}
}

func TestActivityBackfillKeepsAConcurrentLegacyObservation(t *testing.T) {
	pool := bootstrapped(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		ALTER TABLE job_activity DROP CONSTRAINT job_activity_job_id_fkey;
		INSERT INTO jobs (source,external_id,url,title,public_slug,last_seen_at)
		VALUES ('test','concurrent','https://example.test','A','backfill-concurrent','2020-01-01');
		DELETE FROM job_activity;
	`); err != nil {
		t.Fatal(err)
	}
	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(context.Background()) }()
	var writerPID int
	if err := writer.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&writerPID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `UPDATE jobs SET last_seen_at='2026-11-13T00:00:00Z' WHERE external_id='concurrent'`); err != nil {
		t.Fatal(err)
	}
	migrations := activityBackfillMigrations(t, "9999_")
	done := make(chan error, 1)
	go func() {
		_, _, err := migrate.NewRunner(pool).Run(ctx, migrations, false)
		done <- err
	}()
	// The copy took its old jobs snapshot and is now waiting on the bridge's
	// uncommitted activity insertion. Commit only after that interleaving exists.
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid))
			AND datname=current_database())`, writerPID).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("copy did not wait for concurrent bridge: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var seen time.Time
	if err := pool.QueryRow(ctx, `SELECT last_seen_at FROM job_activity`).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if !seen.Equal(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("copy overwrote concurrent observation: %v", seen)
	}
}

func activityBackfillMigrations(t *testing.T, prefix string) []migrate.Migration {
	t.Helper()
	root, err := modroot.Find()
	if err != nil {
		t.Fatal(err)
	}
	all, err := migrate.Load(filepath.Join(root, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var selected []migrate.Migration
	for _, m := range all {
		if strings.HasPrefix(m.Version, "0259_job_activity_") ||
			strings.HasPrefix(m.Version, "0260_job_activity_") ||
			strings.HasPrefix(m.Version, "0261_job_activity_") {
			m.Version = prefix + m.Version
			selected = append(selected, m)
		}
	}
	if len(selected) != 4 {
		t.Fatalf("activity backfill migrations: got %d, want 4", len(selected))
	}
	return selected
}

func TestActivityBackfillIsRecordedBeforeForeignKeyLockRetry(t *testing.T) {
	pool := bootstrapped(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `ALTER TABLE job_activity DROP CONSTRAINT job_activity_job_id_fkey`); err != nil {
		t.Fatal(err)
	}
	locker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = locker.Rollback(context.Background()) }()
	// Ordinary writes allow the copy's reads but conflict with ADD FOREIGN KEY.
	if _, err := locker.Exec(ctx, `LOCK TABLE jobs IN ROW EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	migrations := activityBackfillMigrations(t, "9999_")
	if _, _, err := migrate.NewRunner(pool).Run(ctx, migrations, false); err == nil || !strings.Contains(err.Error(), "lock timeout") {
		t.Fatalf("foreign key DDL while writers hold jobs: %v, want lock timeout", err)
	}
	if !recorded(t, pool, migrations[0].Version) || recorded(t, pool, migrations[1].Version) {
		t.Fatal("copy must be recorded independently of the failed foreign key migration")
	}
	if err := locker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	_, applied, err := migrate.NewRunner(pool).Run(ctx, migrations, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{migrations[1].Version, migrations[2].Version, migrations[3].Version}
	if !slices.Equal(applied, want) {
		t.Fatalf("retry applied %v, want only foreign key steps and validation %v", applied, want)
	}
}
