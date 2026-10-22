//go:build integration

package db

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type gapRow struct {
	Raw, Status, Note string
	Count             int32
	By                int64
}

// backEndGaps lists every row recorded for the specialization "back-end", oldest first.
func backEndGaps(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []gapRow {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT raw_term, status, resolution_note, request_count, submitted_by
		FROM dictionary_gaps WHERE kind = 'specialization' AND normalized_term = 'back-end' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []gapRow
	for rows.Next() {
		var r gapRow
		if err := rows.Scan(&r.Raw, &r.Status, &r.Note, &r.Count, &r.By); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestDictionaryGapKeepsOneLiveRow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	user := seedPlanUser(t, pool, "gap-one@example.test")
	other := seedPlanUser(t, pool, "gap-two@example.test")
	record := func(raw string, by int64) RecordDictionaryGapRow {
		t.Helper()
		row, err := q.RecordDictionaryGap(ctx, RecordDictionaryGapParams{
			Kind: "specialization", RawTerm: raw, NormalizedTerm: "back-end", SubmittedBy: by,
		})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}

	if got := record("Back-End", user); got.RequestCount != 1 || got.Status != "pending" {
		t.Fatalf("first ask = %+v", got)
	}
	if got := record("back end", other); got.RequestCount != 2 || got.Status != "pending" {
		t.Fatalf("second ask = %+v", got)
	}
	rows := backEndGaps(t, ctx, pool)
	if len(rows) != 1 || rows[0].Raw != "Back-End" || rows[0].By != user {
		t.Fatalf("a later ask rewrote the first: %+v", rows)
	}

	if _, err := pool.Exec(ctx, `UPDATE dictionary_gaps SET status = 'accepted' WHERE normalized_term = 'back-end'`); err != nil {
		t.Fatal(err)
	}
	if got := record("back end", other); got.RequestCount != 3 || got.Status != "accepted" {
		t.Fatalf("ask on accepted = %+v", got)
	}

	if _, err := pool.Exec(ctx, `UPDATE dictionary_gaps SET status = 'rejected', resolution_note = 'not a skill' WHERE normalized_term = 'back-end'`); err != nil {
		t.Fatal(err)
	}
	if got := record("back end", other); got.RequestCount != 1 || got.Status != "pending" {
		t.Fatalf("ask after rejection = %+v", got)
	}
	rows = backEndGaps(t, ctx, pool)
	if len(rows) != 2 || rows[0].Status != "rejected" || rows[0].Note != "not a skill" {
		t.Fatalf("rows = %+v, want the rejected one kept beside the new pending", rows)
	}
}

// Two first asks for the same term at once must both count. A read-then-insert
// would let both plan an insert and fail the second on the unique index.
func TestDictionaryGapConcurrentFirstAsks(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	users := []int64{
		seedPlanUser(t, pool, "gap-race-a@example.test"),
		seedPlanUser(t, pool, "gap-race-b@example.test"),
		seedPlanUser(t, pool, "gap-race-c@example.test"),
		seedPlanUser(t, pool, "gap-race-d@example.test"),
	}
	var wg sync.WaitGroup
	counts := make([]int32, len(users))
	errs := make([]error, len(users))
	for i, u := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, err := q.RecordDictionaryGap(ctx, RecordDictionaryGapParams{
				Kind: "specialization", RawTerm: "back end", NormalizedTerm: "back-end", SubmittedBy: u,
			})
			counts[i], errs[i] = row.RequestCount, err
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(counts, func(a, b int) bool { return counts[a] < counts[b] })
	for i, c := range counts {
		if c != int32(i+1) {
			t.Fatalf("counts = %v, want 1..%d", counts, len(users))
		}
	}
}

func TestDictionaryGapDailyCap(t *testing.T) {

	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	user := seedPlanUser(t, pool, "gap-cap@example.test")
	day := pgtype.Date{Time: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC), Valid: true}
	arg := ClaimDictionaryGapWriteParams{UserID: user, Day: day}
	for i := 1; i <= 20; i++ {
		n, err := q.ClaimDictionaryGapWrite(ctx, arg)
		if err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
		if int(n) != i {
			t.Fatalf("claim %d returned %d", i, n)
		}
	}
	if _, err := q.ClaimDictionaryGapWrite(ctx, arg); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("21st claim err = %v, want no row", err)
	}
	var writes int32
	if err := pool.QueryRow(ctx, `SELECT writes FROM dictionary_gap_daily WHERE user_id = $1`, user).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if writes != 20 {
		t.Fatalf("writes = %d, want 20", writes)
	}
}
