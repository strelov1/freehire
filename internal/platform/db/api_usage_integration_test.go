//go:build integration

package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// AddAPIUsage zips its parallel arrays row by row, adds to an existing day rather than
// replacing it (two API processes flush the same day), and skips an account that no
// longer exists instead of failing the whole batch.
func TestAPIUsageAddsAndLists(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	a := seedAccount(t, pool, "usage-a@example.com")
	b := seedAccount(t, pool, "usage-b@example.com")
	d1 := pgtype.Date{Time: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), Valid: true}
	d2 := pgtype.Date{Time: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), Valid: true}

	if err := q.AddAPIUsage(ctx, AddAPIUsageParams{
		UserIds:  []int64{a, a, b, 999999999},
		Days:     []pgtype.Date{d1, d2, d2, d2},
		Requests: []int64{3, 5, 7, 1},
		Errors:   []int64{1, 0, 2, 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.AddAPIUsage(ctx, AddAPIUsageParams{
		UserIds: []int64{a}, Days: []pgtype.Date{d2}, Requests: []int64{10}, Errors: []int64{4},
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := q.ListAPIUsage(ctx, ListAPIUsageParams{UserID: a, Since: d1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Requests != 3 || rows[0].Errors != 1 || rows[1].Requests != 15 || rows[1].Errors != 4 {
		t.Fatalf("account a: got %+v, want day1 3/1 then day2 15/4", rows)
	}
	if !rows[0].Day.Time.Equal(d1.Time) {
		t.Fatalf("oldest first: got %v", rows[0].Day.Time)
	}

	rows, err = q.ListAPIUsage(ctx, ListAPIUsageParams{UserID: a, Since: d2})
	if err != nil || len(rows) != 1 {
		t.Fatalf("since bound: got %+v err %v", rows, err)
	}
}
