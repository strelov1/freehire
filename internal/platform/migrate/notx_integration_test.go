//go:build integration

// Integration tests for the no-transaction path: statements run one at a time, and an
// index left invalid fails the migration instead of recording it. Needs Docker
// (testcontainers); run with: go test -tags=integration ./internal/platform/migrate/
package migrate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/migrate"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// bootstrapped returns a pool the runner treats as already past its baseline, so a
// migration handed to it is APPLIED rather than recorded-without-executing.
func bootstrapped(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatalf("ensure schema_migrations: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO public.schema_migrations (version) VALUES ('0000_bootstrap.sql') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed schema_migrations: %v", err)
	}
	return pool
}

func recorded(t *testing.T, pool *pgxpool.Pool, version string) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM public.schema_migrations WHERE version = $1`, version).Scan(&n); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return n > 0
}

// The marker's whole purpose. Two CONCURRENTLY statements in one file used to go out as a
// single simple query, which Postgres runs as one implicit transaction — and Postgres
// forbids CREATE INDEX CONCURRENTLY inside a transaction block, so this file could not have
// applied at all before the split.
func TestNoTxRunsEachStatementSeparately(t *testing.T) {
	pool := bootstrapped(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE split_probe (id bigint, note text)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}

	migs := []migrate.Migration{{
		Version: "9999_two_concurrent_indexes.sql",
		NoTx:    true,
		SQL: "-- migrate: no-transaction\n" +
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS split_probe_id_idx ON split_probe (id);\n" +
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS split_probe_note_idx ON split_probe (note);\n",
	}}

	if _, applied, err := migrate.NewRunner(pool).Run(ctx, migs, false); err != nil {
		t.Fatalf("Run: %v (applied=%v)", err, applied)
	}

	for _, idx := range []string{"split_probe_id_idx", "split_probe_note_idx"} {
		var valid bool
		if err := pool.QueryRow(ctx,
			`SELECT i.indisvalid FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname = $1`,
			idx).Scan(&valid); err != nil {
			t.Fatalf("read %s: %v", idx, err)
		}
		if !valid {
			t.Errorf("%s exists but is invalid", idx)
		}
	}
}

// The trap this guard exists for, reproduced exactly.
//
// A CREATE INDEX CONCURRENTLY aborted by the runner's lock_timeout leaves the index at
// indisvalid=f. The FIRST run fails loudly and records nothing. The SECOND is the one that
// hurts: `IF NOT EXISTS` sees the carcass, does nothing, reports success, and the version is
// recorded against an index that will never be used — which is what happened to prod on
// 2026-09-03 (see migrations/0126 and 0127). A before-and-after comparison cannot catch it,
// because on the re-run the carcass is in both snapshots.
func TestNoTxRefusesToRecordWhenTheIndexItNamesIsInvalid(t *testing.T) {
	pool := bootstrapped(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE carcass_probe (id bigint)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	// A unique index CONCURRENTLY over duplicate rows fails on its second pass and leaves
	// exactly the carcass a timed-out build leaves.
	if _, err := pool.Exec(ctx, `INSERT INTO carcass_probe (id) VALUES (1), (1)`); err != nil {
		t.Fatalf("seed duplicates: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`CREATE UNIQUE INDEX CONCURRENTLY carcass_probe_id_idx ON carcass_probe (id)`); err == nil {
		t.Fatal("expected the concurrent unique build to fail on duplicate rows")
	}

	const version = "9999_recreate_carcass_probe_idx.sql"
	migs := []migrate.Migration{{
		Version: version,
		NoTx:    true,
		SQL: "-- migrate: no-transaction\n" +
			"CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS carcass_probe_id_idx ON carcass_probe (id);\n",
	}}

	_, applied, err := migrate.NewRunner(pool).Run(ctx, migs, false)
	if err == nil {
		t.Fatalf("Run succeeded over an invalid index; applied=%v", applied)
	}
	if !strings.Contains(err.Error(), "carcass_probe_id_idx") {
		t.Errorf("err = %v, want it to name the invalid index", err)
	}
	if recorded(t, pool, version) {
		t.Error("the version was recorded even though the index it builds is unusable")
	}
}

// Somebody else's older carcass must not fail every run forever — which is why the check
// asks about the indexes THIS FILE names rather than about pg_index as a whole.
func TestNoTxIgnoresAnInvalidIndexItDoesNotName(t *testing.T) {
	pool := bootstrapped(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE unrelated_probe (id bigint)`); err != nil {
		t.Fatalf("create unrelated table: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO unrelated_probe (id) VALUES (1), (1)`); err != nil {
		t.Fatalf("seed duplicates: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`CREATE UNIQUE INDEX CONCURRENTLY unrelated_probe_id_idx ON unrelated_probe (id)`); err == nil {
		t.Fatal("expected the concurrent unique build to fail on duplicate rows")
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE clean_probe (id bigint)`); err != nil {
		t.Fatalf("create clean table: %v", err)
	}

	const version = "9999_clean_probe_idx.sql"
	migs := []migrate.Migration{{
		Version: version,
		NoTx:    true,
		SQL: "-- migrate: no-transaction\n" +
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS clean_probe_id_idx ON clean_probe (id);\n",
	}}

	if _, applied, err := migrate.NewRunner(pool).Run(ctx, migs, false); err != nil {
		t.Fatalf("Run failed over an unrelated carcass: %v (applied=%v)", err, applied)
	}
	if !recorded(t, pool, version) {
		t.Error("the version was not recorded even though its own index built cleanly")
	}
}
