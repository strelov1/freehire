//go:build integration

package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDictionaryGapKeepsOneLiveRow(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	user := seedPlanUser(t, pool, "gap-one@example.test")
	other := seedPlanUser(t, pool, "gap-two@example.test")

	first, err := q.InsertDictionaryGap(ctx, InsertDictionaryGapParams{
		Kind: "specialization", RawTerm: "Back-End", NormalizedTerm: "back-end", SubmittedBy: user,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestCount != 1 || first.Status != "pending" {
		t.Fatalf("insert = %+v", first)
	}
	if _, err := q.InsertDictionaryGap(ctx, InsertDictionaryGapParams{
		Kind: "specialization", RawTerm: "back end", NormalizedTerm: "back-end", SubmittedBy: other,
	}); err == nil {
		t.Fatal("second pending row inserted")
	}

	bumped, err := q.IncrementDictionaryGap(ctx, IncrementDictionaryGapParams{
		Kind: "specialization", NormalizedTerm: "back-end", Status: "pending",
	})
	if err != nil {
		t.Fatal(err)
	}
	if bumped.RequestCount != 2 || bumped.Status != "pending" {
		t.Fatalf("increment = %+v", bumped)
	}
	rows, err := q.ListDictionaryGapsByKey(ctx, ListDictionaryGapsByKeyParams{Kind: "specialization", NormalizedTerm: "back-end"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RawTerm != "Back-End" || rows[0].SubmittedBy != user {
		t.Fatalf("row rewritten: %+v", rows)
	}

	if _, err := pool.Exec(ctx, `UPDATE dictionary_gaps SET status = 'accepted' WHERE id = $1`, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	accepted, err := q.IncrementDictionaryGap(ctx, IncrementDictionaryGapParams{
		Kind: "specialization", NormalizedTerm: "back-end", Status: "accepted",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.RequestCount != 3 || accepted.Status != "accepted" {
		t.Fatalf("accepted increment = %+v", accepted)
	}

	if _, err := pool.Exec(ctx, `UPDATE dictionary_gaps SET status = 'rejected', resolution_note = 'not a skill' WHERE id = $1`, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	again, err := q.InsertDictionaryGap(ctx, InsertDictionaryGapParams{
		Kind: "specialization", RawTerm: "back end", NormalizedTerm: "back-end", SubmittedBy: other,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != "pending" || again.RequestCount != 1 {
		t.Fatalf("reinsert = %+v", again)
	}
	rows, err = q.ListDictionaryGapsByKey(ctx, ListDictionaryGapsByKeyParams{Kind: "specialization", NormalizedTerm: "back-end"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the rejected one kept beside the new pending", len(rows))
	}
	for _, row := range rows {
		if row.Status == "rejected" && row.ResolutionNote != "not a skill" {
			t.Fatalf("rejected note changed: %+v", row)
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
