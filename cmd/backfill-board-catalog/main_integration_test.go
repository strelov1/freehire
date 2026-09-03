//go:build integration

// Run with: go test -tags=integration ./cmd/backfill-board-catalog/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "greenhouse.yml"), []byte(`
- company: Cohere
  board: cohere
- company: Stripe
  board: stripe
`), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBackfillFileInsertsEveryEntryActive(t *testing.T) {
	pool := testdb.Pool(t)
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	ctx := context.Background()
	dir := writeFixture(t)

	inserted, present, failed := backfillFile(ctx, repo, sources.Taxonomy(), filepath.Join(dir, "greenhouse.yml"))
	if inserted != 2 || present != 0 || failed != 0 {
		t.Fatalf("backfillFile = inserted=%d present=%d failed=%d, want 2,0,0", inserted, present, failed)
	}

	rows, err := repo.ListActiveForProvider(ctx, "greenhouse")
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Status != boardcatalog.StatusActive || r.ActivatedAt == nil {
			t.Errorf("row %+v, want status=active with ActivatedAt set", r)
		}
	}
}

// Running the backfill twice must not create duplicate rows — the second run's inserts
// collide with the first run's and are counted as already-present.
func TestBackfillFileIsIdempotent(t *testing.T) {
	pool := testdb.Pool(t)
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	ctx := context.Background()
	dir := writeFixture(t)
	path := filepath.Join(dir, "greenhouse.yml")

	if _, _, failed := backfillFile(ctx, repo, sources.Taxonomy(), path); failed != 0 {
		t.Fatalf("first run: failed = %d, want 0", failed)
	}

	inserted, present, failed := backfillFile(ctx, repo, sources.Taxonomy(), path)
	if inserted != 0 || present != 2 || failed != 0 {
		t.Fatalf("second run = inserted=%d present=%d failed=%d, want 0,2,0", inserted, present, failed)
	}

	rows, err := repo.ListActiveForProvider(ctx, "greenhouse")
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) after second run = %d, want still 2 (no duplicates)", len(rows))
	}
}
