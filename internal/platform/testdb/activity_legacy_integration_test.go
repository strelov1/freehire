//go:build integration

package testdb

import (
	"context"
	"testing"
	"time"
)

// Preserve the original bridge/orphan regressions on release A's real trigger
// bodies. These also verify that historical fixtures still behave as release A
// after the default template has contracted the schema.
func TestJobActivityInsertAndLegacyBridge(t *testing.T) {
	pool := Pool(t)
	InstallLegacyJobActivity(t, pool)
	ctx := context.Background()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug)
 VALUES ('test', 'legacy-bridge', 'https://example.test/legacy', 'A', 'legacy-bridge') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var seen, hydrated time.Time
	var strikes int32
	if err := pool.QueryRow(ctx, `SELECT last_seen_at, liveness_strikes FROM job_activity WHERE job_id=$1`, id).Scan(&seen, &strikes); err != nil {
		t.Fatal(err)
	}
	if time.Since(seen) > time.Minute || strikes != 0 {
		t.Fatalf("insert activity: age=%s strikes=%d", time.Since(seen), strikes)
	}
	future := seen.Add(time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET last_seen_at=$2, hydrated_at=$2, liveness_strikes=2 WHERE id=$1`, id, future); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET last_seen_at=$2, hydrated_at=$2 WHERE id=$1`, id, seen); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT last_seen_at, hydrated_at, liveness_strikes FROM job_activity WHERE job_id=$1`, id).Scan(&seen, &hydrated, &strikes); err != nil {
		t.Fatal(err)
	}
	if !seen.Equal(future) || !hydrated.Equal(future) || strikes != 2 {
		t.Fatalf("bridge regressed observation: seen=%v hydrated=%v strikes=%d", seen, hydrated, strikes)
	}
	// A writer using the old timestamp columns must not overwrite activity-only strikes.
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes=7 WHERE job_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET last_seen_at=last_seen_at + interval '2 hours' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT liveness_strikes FROM job_activity WHERE job_id=$1`, id).Scan(&strikes); err != nil {
		t.Fatal(err)
	}
	if strikes != 7 {
		t.Fatalf("unchanged legacy strikes overwrote activity: %d", strikes)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_activity WHERE job_id=$1`, id).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("activity survived parent deletion")
	}
}

func TestJobActivityInsertReplacesPreConstraintOrphan(t *testing.T) {
	pool := Pool(t)
	InstallLegacyJobActivity(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `ALTER TABLE job_activity DROP CONSTRAINT job_activity_job_id_fkey;
		INSERT INTO job_activity (job_id,last_seen_at,liveness_strikes,state_version)
		VALUES (1001,'2020-01-01',8,2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO jobs (id,source,external_id,url,title,public_slug)
		OVERRIDING SYSTEM VALUE VALUES (1001,'test','reused','https://example.test','A','reused')`); err != nil {
		t.Fatalf("insert reusing an orphan's id during the pre-FK window: %v", err)
	}
	var matches bool
	if err := pool.QueryRow(ctx, `SELECT a.last_seen_at=j.last_seen_at
		AND a.hydrated_at IS NOT DISTINCT FROM j.hydrated_at
		AND a.liveness_strikes=j.liveness_strikes AND a.state_version=3
		FROM jobs j JOIN job_activity a ON a.job_id=j.id WHERE j.id=1001`).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if !matches {
		t.Fatal("a new posting inherited the orphan's stale observation")
	}
}
