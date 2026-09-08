//go:build integration

// Integration tests for the chronic-board mechanism (openspec change
// close-chronically-unreachable-boards, issue #2017): a board that has proven unreachable
// for a long time — not merely cooling down from a recent run of failures — is classified
// chronic so it can be surfaced for curation and, past a second longer window, safety-net
// closed. These are SQL behaviors (first_seen_at's insert-only semantics, the two-window
// classification query), verifiable only against a real Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func truncateBoardHealth(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE board_health"); err != nil {
		t.Fatalf("truncate board_health: %v", err)
	}
}

func boardHealthFirstSeenAt(t *testing.T, pool *pgxpool.Pool, provider, board string) time.Time {
	t.Helper()
	var at time.Time
	if err := pool.QueryRow(context.Background(),
		"SELECT first_seen_at FROM board_health WHERE provider = $1 AND board = $2 AND region = ''",
		provider, board).Scan(&at); err != nil {
		t.Fatalf("read first_seen_at: %v", err)
	}
	return at
}

// seedBoardHealth inserts a board_health row with EXACT timestamps, bypassing
// RecordBoardSuccess/RecordBoardFailure (which always stamp now()) so chronic-window
// boundary tests can pin a board's age precisely. lastSuccessAt nil means the board has
// never succeeded.
func seedBoardHealth(t *testing.T, pool *pgxpool.Pool, provider, board string, firstSeenAt time.Time, lastSuccessAt *time.Time, consecutiveFailures int32) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO board_health (provider, board, region, consecutive_failures, first_seen_at, last_success_at)
		 VALUES ($1, $2, '', $3, $4, $5)`,
		provider, board, consecutiveFailures, firstSeenAt, lastSuccessAt)
	if err != nil {
		t.Fatalf("seed board_health %s/%s: %v", provider, board, err)
	}
}

func daysAgo(d int) time.Time {
	return time.Now().Add(-time.Duration(d) * 24 * time.Hour)
}

// TestRecordBoardSuccessSetsFirstSeenAtOnce pins that first_seen_at is stamped once, on the
// row's first INSERT, and never moves on a later success — it is the anchor a never-yet-
// succeeded board's chronic window is measured from, so a later success (which sets
// last_success_at) must not also reset it.
func TestRecordBoardSuccessSetsFirstSeenAtOnce(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateBoardHealth(t, pool)

	if err := q.RecordBoardSuccess(ctx, RecordBoardSuccessParams{
		Provider: "acme", Board: "b1", Region: "",
		LastIngestedCount: pgtype.Int4{Int32: 5, Valid: true},
	}); err != nil {
		t.Fatalf("record first success: %v", err)
	}
	first := boardHealthFirstSeenAt(t, pool, "acme", "b1")

	if err := q.RecordBoardSuccess(ctx, RecordBoardSuccessParams{
		Provider: "acme", Board: "b1", Region: "",
		LastIngestedCount: pgtype.Int4{Int32: 9, Valid: true},
	}); err != nil {
		t.Fatalf("record second success: %v", err)
	}
	after := boardHealthFirstSeenAt(t, pool, "acme", "b1")

	if !after.Equal(first) {
		t.Fatalf("first_seen_at moved on a later success: got %v, want %v", after, first)
	}
}

// TestRecordBoardFailureDoesNotTouchFirstSeenAt pins the same insert-only invariant across
// the failure path — the row a never-succeeded board accumulates must keep its original
// first_seen_at through every subsequent failure, or the chronic window would keep resetting
// on exactly the boards it exists to catch.
func TestRecordBoardFailureDoesNotTouchFirstSeenAt(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateBoardHealth(t, pool)

	if _, err := q.RecordBoardFailure(ctx, RecordBoardFailureParams{
		Provider: "acme", Board: "b1", Region: "",
		LastError: pgtype.Text{String: "boom", Valid: true},
	}); err != nil {
		t.Fatalf("record first failure: %v", err)
	}
	first := boardHealthFirstSeenAt(t, pool, "acme", "b1")

	for i := 0; i < 3; i++ {
		if _, err := q.RecordBoardFailure(ctx, RecordBoardFailureParams{
			Provider: "acme", Board: "b1", Region: "",
			LastError: pgtype.Text{String: "boom again", Valid: true},
		}); err != nil {
			t.Fatalf("record repeat failure: %v", err)
		}
	}
	after := boardHealthFirstSeenAt(t, pool, "acme", "b1")

	if !after.Equal(first) {
		t.Fatalf("first_seen_at moved across repeated failures: got %v, want %v", after, first)
	}
}

// TestListChronicBoards pins the two-window classification (openspec change
// close-chronically-unreachable-boards, design.md Decision 3): a board is chronic when
// last_success_at is older than the window, or — for a board that has never succeeded —
// when first_seen_at is. A board within the window (whether recently successful or merely
// cooling down for a few days) must not appear, regardless of how many failures it has
// accumulated.
func TestListChronicBoards(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateBoardHealth(t, pool)

	succeededOnce := daysAgo(31)
	neverSucceededFirstSeen := daysAgo(31)
	coolingSince := daysAgo(5)
	justSucceeded := time.Now()

	seedBoardHealth(t, pool, "paylocity", "chronic-once-succeeded", daysAgo(500), &succeededOnce, 27)
	seedBoardHealth(t, pool, "paylocity", "chronic-never-succeeded", neverSucceededFirstSeen, nil, 12)
	seedBoardHealth(t, pool, "paylocity", "cooling-5-days", daysAgo(5), &coolingSince, 5)
	seedBoardHealth(t, pool, "paylocity", "recovered", daysAgo(500), &justSucceeded, 0)

	report, err := q.ListChronicBoards(ctx, ListChronicBoardsParams{
		AgeWindow: pgtype.Interval{Days: 30, Valid: true},
		MaxBoards: 100,
	})
	if err != nil {
		t.Fatalf("list chronic boards (30d): %v", err)
	}
	if got := chronicBoardNames(report); !sameSet(got, []string{"chronic-once-succeeded", "chronic-never-succeeded"}) {
		t.Fatalf("30-day chronic list = %v, want [chronic-once-succeeded chronic-never-succeeded]", got)
	}
	if report[0].Total != int64(len(report)) {
		t.Fatalf("Total = %d, want %d (unlimited by the 100-board cap)", report[0].Total, len(report))
	}

	closure, err := q.ListChronicBoards(ctx, ListChronicBoardsParams{
		AgeWindow: pgtype.Interval{Days: 60, Valid: true},
		MaxBoards: 100,
	})
	if err != nil {
		t.Fatalf("list chronic boards (60d): %v", err)
	}
	if got := chronicBoardNames(closure); len(got) != 0 {
		t.Fatalf("60-day chronic list = %v, want none (31 days short of the 60-day window)", got)
	}
}

// TestListChronicBoardsMaxBoardsCapsButReportsFullTotal pins the cap/total split reused from
// ListUnhealthyBoards: a low cap truncates the returned rows but Total still reports how many
// boards actually qualify, so the caller can tell "these are the worst 1" from "there is only 1".
func TestListChronicBoardsMaxBoardsCapsButReportsFullTotal(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateBoardHealth(t, pool)

	old1 := daysAgo(400)
	old2 := daysAgo(300)
	seedBoardHealth(t, pool, "paylocity", "worst", daysAgo(500), &old1, 50)
	seedBoardHealth(t, pool, "paylocity", "second-worst", daysAgo(500), &old2, 40)

	rows, err := q.ListChronicBoards(ctx, ListChronicBoardsParams{
		AgeWindow: pgtype.Interval{Days: 30, Valid: true},
		MaxBoards: 1,
	})
	if err != nil {
		t.Fatalf("list chronic boards: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 (capped)", len(rows))
	}
	if rows[0].Board != "worst" {
		t.Fatalf("capped row = %q, want the oldest-evidence board %q", rows[0].Board, "worst")
	}
	if rows[0].Total != 2 {
		t.Fatalf("Total = %d, want 2 (both boards qualify, cap only limits rows returned)", rows[0].Total)
	}
}

func chronicBoardNames(rows []ListChronicBoardsRow) []string {
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Board
	}
	return names
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	set := make(map[string]bool, len(want))
	for _, w := range want {
		set[w] = true
	}
	for _, g := range got {
		if !set[g] {
			return false
		}
	}
	return true
}
