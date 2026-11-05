//go:build integration

package emailevents

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestLedger_KeepsTheFirstSightingAndRollsUpAgedEvents(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	record := func(id, kind string, at time.Time) {
		t.Helper()
		if err := q.RecordEmailEvent(ctx, db.RecordEmailEventParams{
			MessageID: id, Type: kind, Campaign: "launch", Variant: "a",
			OccurredAt: pgtype.Timestamptz{Time: at, Valid: true},
		}); err != nil {
			t.Fatalf("RecordEmailEvent: %v", err)
		}
	}

	old := now.Add(-Retention - 24*time.Hour)
	record("m-1", "open", old)
	record("m-1", "open", old.Add(time.Hour)) // a second open changes nothing
	record("m-2", "open", old)
	record("m-3", "open", now)

	if err := RollUp(ctx, q, now); err != nil {
		t.Fatalf("RollUp: %v", err)
	}
	if err := RollUp(ctx, q, now); err != nil {
		t.Fatalf("second RollUp: %v", err)
	}

	var remaining, total int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM email_events`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(count), 0) FROM email_event_daily WHERE campaign = 'launch' AND type = 'open'`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 || total != 2 {
		t.Errorf("remaining = %d, rolled up = %d; want 1 and 2", remaining, total)
	}
}

func TestBroadcastHalves_PartitionTheAudience(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	if _, err := pool.Exec(ctx, `INSERT INTO users (email, email_verified) SELECT 'u' || g || '@example.test', true FROM generate_series(1, 200) g`); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	count := func(half string) int64 {
		t.Helper()
		n, err := q.CountBroadcastCandidates(ctx, db.CountBroadcastCandidatesParams{Campaign: "launch", Half: half})
		if err != nil {
			t.Fatalf("CountBroadcastCandidates: %v", err)
		}
		return n
	}
	all, a, b := count(""), count("a"), count("b")
	if a+b != all || a == 0 || b == 0 {
		t.Errorf("all = %d, a = %d, b = %d; the halves must split the audience", all, a, b)
	}
}
