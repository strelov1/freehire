//go:build integration

// Integration test for the pending -> active transition piggybacked on the board-health
// success write: a board's first successful crawl activates its boards row, using the
// same outcome signal that already updates board_health.
// Run with: go test -tags=integration ./cmd/ingest/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestBoardHealth_RecordSuccessActivatesAPendingBoard(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)
	catalog := boardcatalog.NewQueriesRepository(db.New(pool))

	if _, err := boardcatalog.Insert(ctx, catalog,
		boardcatalog.InsertInput{Provider: "greenhouse", Board: "acme", Company: "Acme"},
		boardcatalog.StatusPending, sources.Taxonomy()); err != nil {
		t.Fatalf("seed pending board: %v", err)
	}

	if err := h.RecordSuccess(ctx, "greenhouse", "acme", "", 3); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	rows, err := catalog.ListActiveForProvider(ctx, "greenhouse")
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != boardcatalog.StatusActive || rows[0].ActivatedAt == nil {
		t.Fatalf("rows = %+v, want one active board with ActivatedAt set", rows)
	}
}

// A success for a board with no boards row (e.g. one crawled before this migration, or a
// board that predates the catalog) must not fail the run — Activate is a best-effort
// no-op when nothing matches.
func TestBoardHealth_RecordSuccessIsHarmlessWithNoMatchingBoard(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)

	if err := h.RecordSuccess(ctx, "greenhouse", "no-catalog-row", "", 1); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
}

// A board already active stays active (and keeps its original ActivatedAt) on a later
// successful crawl — Activate only matches a pending row.
func TestBoardHealth_RecordSuccessDoesNotReactivateAnAlreadyActiveBoard(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)
	catalog := boardcatalog.NewQueriesRepository(db.New(pool))

	first, err := boardcatalog.Insert(ctx, catalog,
		boardcatalog.InsertInput{Provider: "greenhouse", Board: "acme", Company: "Acme"},
		boardcatalog.StatusActive, sources.Taxonomy())
	if err != nil {
		t.Fatalf("seed active board: %v", err)
	}

	if err := h.RecordSuccess(ctx, "greenhouse", "acme", "", 5); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	rows, err := catalog.ListActiveForProvider(ctx, "greenhouse")
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != first.ID || *rows[0].ActivatedAt != *first.ActivatedAt {
		t.Fatalf("rows = %+v, want the same row with its original ActivatedAt unchanged", rows)
	}
}
