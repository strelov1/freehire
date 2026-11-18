//go:build integration

// The savepoint around the candidate mirror, against Postgres: a mirror that fails IN THE
// DATABASE (which aborts a plain transaction) must not take the employer's stage with it.
// Run with: go test -tags=integration ./internal/ingest/directapply/
package directapply

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// failingMirror fails with a real database error on the connection it is given.
type failingMirror struct{ d db.DBTX }

func (m failingMirror) Mirror(ctx context.Context, _, _ int64, _ string) (bool, error) {
	_, err := m.d.Exec(ctx, "SELECT 1/0")
	return false, err
}

func TestSavepointMirror_ADatabaseFailureLeavesTheMoveStanding(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	m := savepointMirror{tx: tx, build: func(d db.DBTX) StageMirror { return failingMirror{d: d} }}
	if _, err := m.Mirror(ctx, 1, 2, "interview"); err == nil {
		t.Fatal("the failing mirror reported success")
	}
	// The transaction is still usable: the failure was contained by the savepoint.
	var one int
	if err := tx.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("a statement after the failed mirror = %d, %v; want the transaction intact", one, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit after a failed mirror: %v", err)
	}
}
