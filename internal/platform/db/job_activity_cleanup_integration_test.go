//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

func TestActivityContractRemovesLegacyColumnsAndKeepsRawInsertion(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	var legacy int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_attribute
		WHERE attrelid = 'jobs'::regclass AND NOT attisdropped
		AND attname IN ('last_seen_at', 'hydrated_at', 'liveness_strikes')`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy != 0 {
		t.Fatalf("jobs still has %d legacy activity columns", legacy)
	}
	var bridge bool
	if err := pool.QueryRow(ctx, `SELECT to_regprocedure('job_activity_legacy_bridge()') IS NOT NULL`).Scan(&bridge); err != nil {
		t.Fatal(err)
	}
	if bridge {
		t.Fatal("the legacy bridge survived contraction")
	}
	id := insertJob(t, pool, "contract-raw-insert")
	var seen time.Time
	var hydrated *time.Time
	var strikes int32
	if err := pool.QueryRow(ctx, `SELECT last_seen_at, hydrated_at, liveness_strikes
		FROM job_activity WHERE job_id = $1`, id).Scan(&seen, &hydrated, &strikes); err != nil {
		t.Fatal(err)
	}
	if time.Since(seen) > time.Minute || hydrated != nil || strikes != 0 {
		t.Fatalf("raw insert observation: seen=%v hydrated=%v strikes=%d", seen, hydrated, strikes)
	}
	var viewSeen time.Time
	if err := pool.QueryRow(ctx, `SELECT last_seen_at FROM jobs_read WHERE id = $1`, id).Scan(&viewSeen); err != nil {
		t.Fatal(err)
	}
	if !viewSeen.Equal(seen) {
		t.Fatalf("view observation %v differs from activity %v", viewSeen, seen)
	}
}
