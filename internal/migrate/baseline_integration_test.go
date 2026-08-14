//go:build integration

// Integration test for the auto-baseline path — the single scenario this whole package
// exists for, per its own package doc comment: a database whose schema already carries
// every migration's effect (an initdb- or hand-migrated volume) but has no
// schema_migrations bookkeeping yet. Only the pure decide() routing function was
// previously unit-tested with a mocked legacySchema bool; hasJobsTable's live
// to_regclass('public.jobs') probe, wired into the full Runner.Run flow, had no coverage
// against a real Postgres — and the package's own lock-timeout integration test
// deliberately seeds schema_migrations first specifically to AVOID this path. Needs
// Docker (testcontainers); run with:
//
//	go test -tags=integration ./internal/migrate/
package migrate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/strelov1/freehire/internal/migrate"
	"github.com/strelov1/freehire/internal/testdb"
)

// repoRoot walks up from the test's working directory to the module root, so this test
// can Load() the real on-disk migrations/*.sql regardless of how deep the package sits.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}

// TestRun_AutoBaselinesAPreRunnerDatabase reproduces the real scenario against a real
// Postgres: testdb.Pool hands out a database whose schema was applied by raw initdb
// scripts (every migrations/*.sql file executed directly — the same mechanism a fresh
// prod volume bootstraps with) — so public.jobs exists, but nothing has ever created
// schema_migrations. That is byte-for-byte "an initdb- or hand-migrated volume" from the
// package doc comment. Run() against this database, with the real on-disk migration set,
// must recognize the legacy schema and BASELINE every file (record it as applied without
// re-executing it) rather than replay DDL that already ran — replaying would fail
// outright (e.g. a duplicate CREATE TABLE), which is exactly the failure this package
// exists to avoid on a prod bootstrap.
func TestRun_AutoBaselinesAPreRunnerDatabase(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	migs, err := migrate.Load(filepath.Join(repoRoot(t), "migrations"))
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migs) == 0 {
		t.Fatal("loaded zero migrations — the on-disk directory is unexpectedly empty")
	}

	var alreadyExists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&alreadyExists); err != nil {
		t.Fatalf("probe schema_migrations: %v", err)
	}
	if alreadyExists {
		t.Fatal("schema_migrations already exists before Run — testdb.Pool no longer hands out a pre-runner database, this test needs rethinking")
	}

	baselined, applied, err := migrate.NewRunner(pool).Run(ctx, migs, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("applied = %v, want nothing executed — every file's effect is already on disk", applied)
	}
	if len(baselined) != len(migs) {
		t.Fatalf("baselined %d migration(s), want all %d recorded without executing them", len(baselined), len(migs))
	}

	var recorded int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM public.schema_migrations").Scan(&recorded); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if recorded != len(migs) {
		t.Fatalf("schema_migrations has %d row(s), want one per on-disk migration (%d)", recorded, len(migs))
	}

	// A second run against the now-baselined database must be a true no-op: nothing
	// legacy about it anymore (schema_migrations is no longer empty), and nothing
	// pending either.
	baselined2, applied2, err := migrate.NewRunner(pool).Run(ctx, migs, false)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(baselined2) != 0 || len(applied2) != 0 {
		t.Fatalf("second Run baselined=%v applied=%v, want both empty", baselined2, applied2)
	}
}
