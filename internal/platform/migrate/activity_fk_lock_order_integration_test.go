//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/platform/migrate"
	"github.com/strelov1/freehire/internal/platform/modroot"
)

// The foreign key is added after the copy, and ADD FOREIGN KEY locks the child (job_activity)
// before the parent (jobs), both SHARE ROW EXCLUSIVE. A legacy writer takes them the other way
// round: UPDATE jobs (ROW EXCLUSIVE), then the bridge trigger writes job_activity (ROW
// EXCLUSIVE). Run against a live writer the two form a cycle: the DDL holds the child and waits
// for the parent, the writer holds the parent and waits for the child. Production hit exactly
// this on 2026-11-13 (40P01 on the migration and on 14 writers), with 0259 already recorded.
//
// The test is the same on both sides of the fix: it applies every migration that follows the
// recorded copy, against a writer that holds jobs and then reaches for job_activity once the DDL
// is queued, and demands that nobody is chosen as a deadlock victim.
func TestForeignKeyMigrationDoesNotDeadlockWithABridgeWriter(t *testing.T) {
	pool := bootstrapped(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Production after 0259: the copy is done and recorded, the constraint does not exist yet.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE job_activity DROP CONSTRAINT job_activity_job_id_fkey;
		INSERT INTO jobs (source, external_id, url, title, public_slug, last_seen_at)
		VALUES ('test', 'fk-order', 'https://example.test/fk', 'A', 'fk-order', '2020-01-01');
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
	// ROW EXCLUSIVE on jobs, without touching job_activity yet.
	if _, err := writer.Exec(ctx, `UPDATE jobs SET title = 'B' WHERE external_id = 'fk-order'`); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, _, err := migrate.NewRunner(pool).Run(ctx, foreignKeyMigrations(t, "9999_"), false)
		done <- err
	}()
	// The DDL is now queued behind the writer. Whatever it already holds, the writer's next
	// statement meets it.
	for {
		var queued bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid))
			AND datname = current_database())`, writerPID).Scan(&queued); err != nil {
			t.Fatal(err)
		}
		if queued {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("migration finished without waiting for the writer: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}

	logTableLocks(t, ctx, pool, "before the writer reaches job_activity")

	// The bridge trigger writes job_activity inside this statement.
	_, writerErr := writer.Exec(ctx, `UPDATE jobs SET last_seen_at = '2026-11-13T00:00:00Z' WHERE external_id = 'fk-order'`)
	if writerErr == nil {
		writerErr = writer.Commit(ctx)
	} else {
		_ = writer.Rollback(ctx)
	}
	migrationErr := <-done

	for who, err := range map[string]error{"writer": writerErr, "migration": migrationErr} {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "40P01" {
			t.Errorf("%s was chosen as a deadlock victim: %v", who, err)
		}
	}
	if writerErr != nil || migrationErr != nil {
		t.Fatalf("writer: %v; migration: %v", writerErr, migrationErr)
	}
	var fk, validated bool
	if err := pool.QueryRow(ctx, `SELECT true, convalidated FROM pg_constraint
		WHERE conrelid = 'job_activity'::regclass AND conname = 'job_activity_job_id_fkey'`).Scan(&fk, &validated); err != nil || !fk || !validated {
		t.Fatalf("foreign key present=%v validated=%v err=%v", fk, validated, err)
	}
	var seen time.Time
	if err := pool.QueryRow(ctx, `SELECT last_seen_at FROM job_activity`).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if !seen.Equal(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("the writer's observation was lost: %v", seen)
	}
}

// foreignKeyMigrations is everything release A applies after the recorded copy (0259 backfill):
// the foreign key (one file or more) and its validation, under a throwaway version prefix.
func foreignKeyMigrations(t *testing.T, prefix string) []migrate.Migration {
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
		if m.Version == "0259_job_activity_backfill.sql" {
			continue
		}
		if strings.HasPrefix(m.Version, "0259_job_activity_") ||
			strings.HasPrefix(m.Version, "0260_job_activity_") ||
			strings.HasPrefix(m.Version, "0261_job_activity_") {
			m.Version = prefix + m.Version
			selected = append(selected, m)
		}
	}
	if len(selected) < 2 {
		t.Fatalf("foreign key migrations: got %d", len(selected))
	}
	return selected
}

// logTableLocks prints who holds and who waits on the two tables, so a failure shows the cycle.
func logTableLocks(t *testing.T, ctx context.Context, pool interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, when string) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT l.pid, l.relation::regclass::text, l.mode, l.granted
		FROM pg_locks l JOIN pg_stat_activity a USING (pid)
		WHERE l.locktype = 'relation' AND l.relation IN ('jobs'::regclass, 'job_activity'::regclass)
		  AND a.datname = current_database() AND l.pid <> pg_backend_pid() ORDER BY 2, 4 DESC, 3`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var pid int
		var rel, mode string
		var granted bool
		if err := rows.Scan(&pid, &rel, &mode, &granted); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: pid=%d %s %s granted=%v", when, pid, rel, mode, granted)
	}
}

func TestForeignKeyCorrectionDoesNotBlockWritersWhenTheConstraintExists(t *testing.T) {
	pool := bootstrapped(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(context.Background()) }()
	if _, err := writer.Exec(ctx, `LOCK TABLE jobs IN ROW EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	migrations := foreignKeyMigrations(t, "9999_")
	if !strings.Contains(migrations[0].Version, "0259_job_activity_foreign_key_lock_order.sql") {
		t.Fatalf("first migration must repair FK lock ordering: %s", migrations[0].Version)
	}
	if _, _, err := migrate.NewRunner(pool).Run(ctx, migrations[:1], false); err != nil {
		t.Fatalf("an existing constraint must not require a jobs DDL lock: %v", err)
	}
}
