//go:build integration

// Integration test for the migration runner's lock timeout. Needs Docker (testcontainers);
// run with: go test -tags=integration ./internal/migrate/
package migrate_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/migrate"
	"github.com/strelov1/freehire/internal/testdb"
)

// A migration that needs ACCESS EXCLUSIVE must fail rather than queue for it.
//
// This reproduces a real incident (2026-07-30). The nightly pg_dump holds ACCESS SHARE on
// every table for the length of the dump. An `ALTER TABLE` issued in that window cannot get
// ACCESS EXCLUSIVE, and while it waits it sits at the head of the table's lock queue — so
// every ordinary reader that arrives behind it also waits. One bounded schema change became
// minutes of hanging profile reads for signed-in users. Failing fast keeps the release
// honest: release.sh aborts with the live colour untouched, and nobody's read is blocked.
func TestRun_FailsFastWhenAnotherTransactionHoldsTheTableLock(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `CREATE TABLE lock_probe (id bigint)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	// A recorded version makes the runner treat this database as already bootstrapped, so the
	// migration below is APPLIED rather than baselined (the legacy-schema path records without
	// executing, which would not exercise the lock at all).
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatalf("ensure schema_migrations: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO public.schema_migrations (version) VALUES ('0000_bootstrap.sql')
		ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed schema_migrations: %v", err)
	}

	// Hold ACCESS SHARE on the probe table, exactly as a running pg_dump would.
	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire holder: %v", err)
	}
	defer holder.Release()
	tx, err := holder.Begin(ctx)
	if err != nil {
		t.Fatalf("begin holder tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT count(*) FROM lock_probe`); err != nil {
		t.Fatalf("holder read: %v", err)
	}

	migs := []migrate.Migration{{
		Version: "9999_needs_access_exclusive.sql",
		SQL:     `ALTER TABLE lock_probe ADD COLUMN note text`,
	}}

	start := time.Now()
	_, applied, err := migrate.NewRunner(pool).Run(ctx, migs, false)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("Run succeeded while the table lock was held; applied=%v", applied)
	}
	if !strings.Contains(err.Error(), "lock timeout") {
		t.Errorf("err = %v, want a lock-timeout failure", err)
	}
	// The point is that it gives up quickly instead of queueing for the holder. Generous
	// enough not to flake on a slow CI container, far below "waits for the dump".
	if elapsed > 60*time.Second {
		t.Errorf("waited %s for the lock — the runner must fail fast, not queue", elapsed.Round(time.Second))
	}
	if len(applied) != 0 {
		t.Errorf("applied = %v, want nothing recorded when the migration could not run", applied)
	}
}
