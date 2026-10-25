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

	if _, err := q.RecordBoardSuccess(ctx, RecordBoardSuccessParams{
		Provider: "acme", Board: "b1", Region: "",
		LastIngestedCount: pgtype.Int4{Int32: 5, Valid: true},
	}); err != nil {
		t.Fatalf("record first success: %v", err)
	}
	first := boardHealthFirstSeenAt(t, pool, "acme", "b1")

	if _, err := q.RecordBoardSuccess(ctx, RecordBoardSuccessParams{
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

// TestBoardHealthIdentityIsCaseInsensitive pins the invariant migration 0171 replaced the
// old application-level guard with: board_health's identity key is (provider,
// lower(board), region), so two rows that differ only by the board's casing cannot coexist
// — a direct INSERT of the second casing must fail the unique index, the same way a second
// insert of the exact identity would.
//
// This replaces TestListChronicBoardsIgnoresAStaleCaseTwin and
// TestListChronicBoardsKeepsBothWhenNeitherTwinIsFresher, which pinned the NOT EXISTS
// subquery ListChronicBoards used to carry to hide a stale case twin from the chronic
// report (see 0170's incident writeup). That guard, and the twin rows it was written
// against, could only ever exist because the key itself was case-sensitive; 0171 made the
// twin impossible to create in the first place, so there is nothing left for a query-level
// guard to compensate for.
func TestBoardHealthIdentityIsCaseInsensitive(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	truncateBoardHealth(t, pool)

	stale := daysAgo(90)
	seedBoardHealth(t, pool, "smartrecruiters", "atlas4", daysAgo(500), &stale, 3)

	_, err := pool.Exec(ctx,
		`INSERT INTO board_health (provider, board, region, first_seen_at) VALUES ($1, $2, '', now())`,
		"smartrecruiters", "ATLAS4")
	if err == nil {
		t.Fatal("inserting a differently-cased twin succeeded, want a unique-index violation")
	}
}

// TestRecordBoardSuccessConvergesOnAnExistingRowUnderANewCasing pins the upsert half of the
// same fix: RecordBoardSuccess/RecordBoardFailure now conflict on (provider, lower(board),
// region) and write board = EXCLUDED.board, so a board id that changes case at the provider
// updates the EXISTING row — including its stored spelling — instead of inserting a second
// one. Without `board = EXCLUDED.board` the row would keep its original casing forever, and
// every later exact-match lookup by the provider's current casing (GetBoardCooldown,
// SetBoardCooldown, DeleteBoardHealth, ClearProviderCooldowns) would stop finding it.
func TestRecordBoardSuccessConvergesOnAnExistingRowUnderANewCasing(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateBoardHealth(t, pool)

	if _, err := q.RecordBoardFailure(ctx, RecordBoardFailureParams{
		Provider: "smartrecruiters", Board: "atlas4", Region: "",
		LastError: pgtype.Text{String: "boom", Valid: true},
	}); err != nil {
		t.Fatalf("record failure under the old casing: %v", err)
	}

	if _, err := q.RecordBoardSuccess(ctx, RecordBoardSuccessParams{
		Provider: "smartrecruiters", Board: "ATLAS4", Region: "",
		LastIngestedCount: pgtype.Int4{Int32: 5, Valid: true},
	}); err != nil {
		t.Fatalf("record success under the new casing: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM board_health WHERE provider = $1 AND lower(board) = lower($2) AND region = ''",
		"smartrecruiters", "atlas4").Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1 (the success upsert must converge onto the existing row)", count)
	}

	var storedBoard string
	var failures int32
	if err := pool.QueryRow(ctx,
		"SELECT board, consecutive_failures FROM board_health WHERE provider = $1 AND lower(board) = lower($2) AND region = ''",
		"smartrecruiters", "atlas4").Scan(&storedBoard, &failures); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if storedBoard != "ATLAS4" {
		t.Fatalf("stored board = %q, want %q: the upsert must adopt the new casing, not freeze the first one seen", storedBoard, "ATLAS4")
	}
	if failures != 0 {
		t.Fatalf("consecutive_failures = %d, want 0: the success upsert must have cleared the failure state on the SAME row", failures)
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

// TestSetBoardCooldownGuardsOnConsecutiveFailures pins the CAS guard that keeps two
// concurrent RecordFailure calls for the same board from applying their cooldowns out of
// order (the pipeline's worker pool can legitimately process one board twice in a run). A
// call whose expected consecutive_failures no longer matches the stored value — a newer
// writer already moved it — affects zero rows and must not touch cooldown_until; a call
// whose expected value still matches applies exactly as before.
func TestSetBoardCooldownGuardsOnConsecutiveFailures(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncateBoardHealth(t, pool)

	if _, err := q.RecordBoardFailure(ctx, RecordBoardFailureParams{
		Provider: "greenhouse", Board: "acme", Region: "",
		LastError: pgtype.Text{String: "boom", Valid: true},
	}); err != nil {
		t.Fatalf("seed failure: %v", err)
	}

	// Truncated to microseconds: that is all timestamptz stores, so an untruncated
	// nanosecond-precision time.Now() round-trips through Postgres with its last digits
	// rounded away, and a direct comparison against the pre-truncation value fails even
	// though both name the same cooldown.
	staleCooldown := pgtype.Timestamptz{Time: time.Now().Add(time.Hour).Truncate(time.Microsecond), Valid: true}
	rows, err := q.SetBoardCooldown(ctx, SetBoardCooldownParams{
		Provider: "greenhouse", Board: "acme", Region: "",
		CooldownUntil:       staleCooldown,
		ConsecutiveFailures: 99, // does not match the actual stored value (1)
	})
	if err != nil {
		t.Fatalf("SetBoardCooldown (mismatched guard): %v", err)
	}
	if rows != 0 {
		t.Fatalf("rows affected = %d, want 0 for a mismatched expected consecutive_failures", rows)
	}
	if until, ok, err := getCooldown(ctx, q, "greenhouse", "acme"); err != nil {
		t.Fatalf("read cooldown after mismatched guard: %v", err)
	} else if ok {
		t.Fatalf("cooldown_until = %v, want still NULL — a mismatched guard must write nothing", until)
	}

	freshCooldown := pgtype.Timestamptz{Time: time.Now().Add(2 * time.Hour).Truncate(time.Microsecond), Valid: true}
	rows, err = q.SetBoardCooldown(ctx, SetBoardCooldownParams{
		Provider: "greenhouse", Board: "acme", Region: "",
		CooldownUntil:       freshCooldown,
		ConsecutiveFailures: 1, // matches the actual stored value
	})
	if err != nil {
		t.Fatalf("SetBoardCooldown (matching guard): %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows affected = %d, want 1 for a matching expected consecutive_failures", rows)
	}
	if until, ok, err := getCooldown(ctx, q, "greenhouse", "acme"); err != nil {
		t.Fatalf("read cooldown after matching guard: %v", err)
	} else if !ok || !until.Equal(freshCooldown.Time) {
		t.Fatalf("cooldown_until = (%v, %v), want (%v, true)", until, ok, freshCooldown.Time)
	}
}

func getCooldown(ctx context.Context, q *Queries, provider, board string) (time.Time, bool, error) {
	row, err := q.GetBoardCooldown(ctx, GetBoardCooldownParams{Provider: provider, Board: board, Region: ""})
	if err != nil {
		return time.Time{}, false, err
	}
	ts := row.CooldownUntil
	if !ts.Valid {
		return time.Time{}, false, nil
	}
	return ts.Time, true, nil
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
