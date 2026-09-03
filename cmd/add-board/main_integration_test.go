//go:build integration

// Run with: go test -tags=integration ./cmd/add-board/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
//
// Tests call addBoard/retireBoard directly (not runAdd/runRetire) so the database under
// test is testdb's throwaway container, not whatever DATABASE_URL the environment has —
// runAdd/runRetire's own worker.Bootstrap call is CLI-facing production wiring, out of
// reach of a test that wants a specific, disposable database.
package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestAddBoardInsertsActive(t *testing.T) {
	pool := testdb.Pool(t)
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	in := boardcatalog.InsertInput{Provider: "greenhouse", Board: "acme", Company: "Acme"}

	if code := addBoard(context.Background(), pool, in, sources.Taxonomy()); code != 0 {
		t.Fatalf("addBoard exit = %d, want 0", code)
	}

	rows, err := repo.ListActiveForProvider(context.Background(), "greenhouse")
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != boardcatalog.StatusActive || rows[0].ActivatedAt == nil {
		t.Fatalf("rows = %+v, want one active row with ActivatedAt set", rows)
	}
}

// Re-adding the same identity is refused, not a second row — the same
// boards_identity_key that guards a crowdsourced duplicate.
func TestAddBoardIsANoOpOnReRun(t *testing.T) {
	pool := testdb.Pool(t)
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	in := boardcatalog.InsertInput{Provider: "lever", Board: "acme", Company: "Acme"}

	if code := addBoard(context.Background(), pool, in, sources.Taxonomy()); code != 0 {
		t.Fatalf("first addBoard exit = %d, want 0", code)
	}
	if code := addBoard(context.Background(), pool, in, sources.Taxonomy()); code == 0 {
		t.Fatal("second addBoard (duplicate) exit = 0, want nonzero")
	}

	rows, err := repo.ListActiveForProvider(context.Background(), "lever")
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want still exactly 1 (no duplicate)", rows)
	}
}

// runAdd itself refuses an invalid candidate (unknown provider) before ever opening a
// database connection — proven by exercising the real CLI-facing runAdd, which would
// otherwise fail on the environment's DATABASE_URL rather than reach this check.
func TestRunAddRejectsUnknownProviderBeforeTouchingTheDatabase(t *testing.T) {
	if code := runAdd("no-such-provider", "acme", "", "Acme", false, "", true); code == 0 {
		t.Fatal("runAdd with unknown provider exit = 0, want nonzero")
	}
}

func TestRetireBoardRetiresWithoutDeleting(t *testing.T) {
	pool := testdb.Pool(t)
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	in := boardcatalog.InsertInput{Provider: "gem", Board: "acme", Company: "Acme"}
	if code := addBoard(context.Background(), pool, in, sources.Taxonomy()); code != 0 {
		t.Fatalf("seed addBoard: exit = %d", code)
	}

	if code := retireBoard(context.Background(), pool, "gem", "acme", ""); code != 0 {
		t.Fatalf("retireBoard exit = %d, want 0", code)
	}

	rows, err := repo.ListActiveForProvider(context.Background(), "gem")
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none live after retirement", rows)
	}

	var status string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM boards WHERE provider = 'gem' AND board = 'acme'`).Scan(&status); err != nil {
		t.Fatalf("row should still exist (retired, not deleted): %v", err)
	}
	if status != string(boardcatalog.StatusRetired) {
		t.Errorf("status = %q, want %q", status, boardcatalog.StatusRetired)
	}
}

func TestRenameBoardCorrectsThePlaceholder(t *testing.T) {
	pool := testdb.Pool(t)
	repo := boardcatalog.NewQueriesRepository(db.New(pool))
	in := boardcatalog.InsertInput{Provider: "greenhouse", Board: "acme-corp", Company: boardcatalog.PlaceholderCompany("acme-corp")}
	if code := addBoard(context.Background(), pool, in, sources.Taxonomy()); code != 0 {
		t.Fatalf("seed addBoard: exit = %d", code)
	}

	if code := renameBoard(context.Background(), pool, "greenhouse", "acme-corp", "", "Acme Corporation Inc."); code != 0 {
		t.Fatalf("renameBoard exit = %d, want 0", code)
	}

	rows, err := repo.ListActiveForProvider(context.Background(), "greenhouse")
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 1 || rows[0].Company != "Acme Corporation Inc." {
		t.Fatalf("rows = %+v, want the renamed company", rows)
	}
}

func TestRenameBoardReportsNotFound(t *testing.T) {
	pool := testdb.Pool(t)

	if code := renameBoard(context.Background(), pool, "greenhouse", "no-such-board", "", "New Name"); code == 0 {
		t.Fatal("renameBoard on a nonexistent board exit = 0, want nonzero")
	}
}

func TestRetireBoardReportsNotFound(t *testing.T) {
	pool := testdb.Pool(t)

	if code := retireBoard(context.Background(), pool, "greenhouse", "no-such-board", ""); code == 0 {
		t.Fatal("retireBoard on a nonexistent board exit = 0, want nonzero")
	}
}
