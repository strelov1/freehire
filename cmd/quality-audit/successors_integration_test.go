//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// The retirement re-checks the old board under a lock: a board still gone is retired, one that
// recovered after the network pass is left alone.
func TestTxApplierRechecksAtTheWrite(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	for _, b := range []string{"old", "recovered", "new"} {
		if _, err := q.InsertBoard(ctx, db.InsertBoardParams{
			Provider: "trakstar", Board: b, Company: "Acme", Tenants: []byte("{}"), Status: "active", Surface: "curator",
		}); err != nil {
			t.Fatalf("seed %s: %v", b, err)
		}
	}
	for _, b := range []string{"old", "recovered"} {
		if _, err := q.RecordBoardFailure(ctx, db.RecordBoardFailureParams{
			Provider: "trakstar", Board: b,
			LastError: pgtype.Text{String: "trakstar: fetch board " + b + ": status 404", Valid: true},
		}); err != nil {
			t.Fatalf("fail %s: %v", b, err)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE board_health SET first_seen_at = now() - interval '10 days' WHERE provider = 'trakstar'`); err != nil {
		t.Fatal(err)
	}
	// "recovered" was crawled successfully after the pass saw it fail.
	if _, err := q.RecordBoardSuccess(ctx, db.RecordBoardSuccessParams{
		Provider: "trakstar", Board: "recovered", LastIngestedCount: pgtype.Int4{Int32: 3, Valid: true}, Reached: true,
	}); err != nil {
		t.Fatal(err)
	}

	applier := txApplier{pool: pool, window: 7 * 24 * time.Hour}
	move := func(board string) successorMove {
		return successorMove{Provider: "trakstar", Board: board, SuccessorProvider: "trakstar", SuccessorBoard: "new"}
	}

	r, held, err := applier.RetireIfStillMoved(ctx, move("old"))
	if err != nil || !held || !r.Retired {
		t.Fatalf("old: retirement=%+v held=%v err=%v, want retired", r, held, err)
	}
	if _, held, err = applier.RetireIfStillMoved(ctx, move("recovered")); err != nil || held {
		t.Fatalf("recovered: held=%v err=%v, want left alone", held, err)
	}

	status := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT board, status FROM boards WHERE provider = 'trakstar'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var b, s string
		if err := rows.Scan(&b, &s); err != nil {
			t.Fatal(err)
		}
		status[b] = s
	}
	if status["old"] != "retired" || status["recovered"] != "active" || status["new"] != "active" {
		t.Fatalf("statuses = %v", status)
	}
}
