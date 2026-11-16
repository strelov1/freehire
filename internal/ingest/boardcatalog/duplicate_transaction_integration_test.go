//go:build integration

package boardcatalog

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestDuplicateBoardInsertKeepsTheCallerTransactionUsable(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	repo := NewQueriesRepository(db.New(tx))
	in := InsertInput{Provider: "hrsmart", Board: "gistworld.mua.hrdepartment.com", Company: "Gist Limited"}
	if _, err := repo.InsertRow(ctx, in, StatusPending, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertRow(ctx, in, StatusPending, ""); !errors.Is(err, ErrDuplicateBoard) {
		t.Fatalf("duplicate: %v", err)
	}
	var n int
	if err := tx.QueryRow(ctx, "SELECT 1").Scan(&n); err != nil || n != 1 {
		t.Fatalf("duplicate aborted its caller transaction: %v", err)
	}
}
