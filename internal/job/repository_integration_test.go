//go:build integration

// Integration tests for the Job aggregate's repository: loading by dedup identity
// and soft-closing by it, verified against a real Postgres. Run with:
// go test -tags=integration ./internal/job/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package job

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/strelov1/freehire/internal/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	scripts, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("list migrations: %v (found %d)", err, len(scripts))
	}
	sort.Strings(scripts)

	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("hire"),
		postgres.WithUsername("hire"),
		postgres.WithPassword("hire"),
		postgres.WithInitScripts(scripts...),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestQueriesRepository_LoadAndClose(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := NewQueriesRepository(db.New(pool))

	// Seed a job with facets and enrichment straight into the table.
	// created_by is left null here (it FKs to users, out of scope); the
	// ManuallyAdded mapping is covered by the jobFromRow unit test.
	_, err := pool.Exec(ctx,
		`INSERT INTO jobs (source, external_id, url, title, company, company_slug, public_slug,
		                   location, countries, skills, seniority, category, enrichment)
		 VALUES ('greenhouse', 'acme:42', 'http://x.test', 'Senior Go Developer', 'Acme', 'acme',
		         'senior-go-developer-acme-1', 'Berlin, Germany', ARRAY['de'], ARRAY['go','postgresql'],
		         'senior', 'backend', '{"summary":"Great","countries":["es"]}'::jsonb)`)
	if err != nil {
		t.Fatalf("seed job: %v", err)
	}
	// Bump the materialized engagement counters so Extras has something to carry.
	if _, err := pool.Exec(ctx,
		`UPDATE jobs SET view_count = 3, applied_count = 1 WHERE source='greenhouse' AND external_id='acme:42'`); err != nil {
		t.Fatalf("bump counters: %v", err)
	}

	j, x, err := repo.Load(ctx, "greenhouse", "acme:42")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if x.ViewCount != 3 || x.AppliedCount != 1 {
		t.Errorf("Extras counters = %d/%d, want 3/1", x.ViewCount, x.AppliedCount)
	}
	f := j.Fields()
	if f.Title != "Senior Go Developer" || f.Seniority != "senior" || f.Category != "backend" {
		t.Errorf("loaded facets = %q/%q/%q", f.Title, f.Seniority, f.Category)
	}
	if len(f.Skills) != 2 || f.Enrichment.Summary != "Great" {
		t.Errorf("loaded skills=%v enrichment=%+v", f.Skills, f.Enrichment)
	}
	if !j.IsOpen() {
		t.Error("freshly seeded job should load as open")
	}

	// Close by identity, then reload — the aggregate reflects the closed state.
	n, err := repo.Close(ctx, "greenhouse", "acme:42")
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n != 1 {
		t.Errorf("Close affected %d rows, want 1", n)
	}
	reloaded, _, err := repo.Load(ctx, "greenhouse", "acme:42")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.IsOpen() {
		t.Error("job should load as closed after Close")
	}

	// Closing an already-closed job is a no-op (0 rows).
	if n, _ := repo.Close(ctx, "greenhouse", "acme:42"); n != 0 {
		t.Errorf("second Close affected %d rows, want 0", n)
	}
}

func TestQueriesRepository_LoadNotFound(t *testing.T) {
	pool := startPostgres(t)
	repo := NewQueriesRepository(db.New(pool))
	if _, _, err := repo.Load(context.Background(), "nope", "missing"); err != ErrNotFound {
		t.Errorf("Load(missing) err = %v, want ErrNotFound", err)
	}
}
