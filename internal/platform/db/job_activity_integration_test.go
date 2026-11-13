//go:build integration

package db

import (
	"context"
	"testing"
	"time"
)

func TestJobActivityInsertAndLegacyBridge(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	id := insertJob(t, pool, "activity")
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

func TestJobActivityStateTracksContentAndReopening(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	id := insertJob(t, pool, "activity-state")
	changes := []string{
		`content_hash='changed'`, `cities=ARRAY['berlin']`,
		`salary_min_source=100`, `salary_max_source=200`,
		`salary_currency_source='EUR'`, `salary_period_source='year'`,
		`english_level='b2'`, `is_tech=false`, `engagement=ARRAY['part_time']`,
		`application_deadline=now()+interval '1 day'`, `closed_at=now()`,
		`closed_at=NULL`,
		`is_private=true`, `is_private=false`,
	}
	var previous int64
	for _, change := range changes {
		if _, err := pool.Exec(ctx, `UPDATE jobs SET `+change+` WHERE id=$1`, id); err != nil {
			t.Fatalf("%s: %v", change, err)
		}
		var version int64
		if err := pool.QueryRow(ctx, `SELECT state_version FROM job_activity WHERE job_id=$1`, id).Scan(&version); err != nil {
			t.Fatal(err)
		}
		if version != previous+1 {
			t.Fatalf("%s: version=%d, previous=%d", change, version, previous)
		}
		previous = version
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at=now(), hydrated_at=now(), liveness_strikes=4 WHERE job_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET view_count=view_count+1, updated_at=now() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var version int64
	if err := pool.QueryRow(ctx, `SELECT state_version FROM job_activity WHERE job_id=$1`, id).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != previous {
		t.Fatalf("observation/counter write moved state version: %d -> %d", previous, version)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at=now() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at=NULL WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var strikes int32
	if err := pool.QueryRow(ctx, `SELECT liveness_strikes FROM job_activity WHERE job_id=$1`, id).Scan(&strikes); err != nil {
		t.Fatal(err)
	}
	if strikes != 0 {
		t.Fatalf("reopen did not reset strikes: %d", strikes)
	}
}

func TestJobActivityStateTracksDuplicateOwners(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	canonical := insertJob(t, pool, "activity-canonical")
	for _, column := range []string{"duplicate_of_role", "duplicate_of_aggregator", "duplicate_of_fuzzy"} {
		t.Run(column, func(t *testing.T) {
			id := insertJob(t, pool, column)
			if _, err := pool.Exec(ctx, `UPDATE jobs SET `+column+`=$2 WHERE id=$1`, id, canonical); err != nil {
				t.Fatal(err)
			}
			var version int64
			if err := pool.QueryRow(ctx, `SELECT state_version FROM job_activity WHERE job_id=$1`, id).Scan(&version); err != nil {
				t.Fatal(err)
			}
			if version != 1 {
				t.Fatalf("effective duplicate changed but state_version=%d, want 1", version)
			}
			if _, err := pool.Exec(ctx, `UPDATE jobs SET `+column+`=NULL WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT state_version FROM job_activity WHERE job_id=$1`, id).Scan(&version); err != nil {
				t.Fatal(err)
			}
			if version != 2 {
				t.Fatalf("cleared effective duplicate but state_version=%d, want 2", version)
			}
		})
	}
}

func TestJobActivityInsertReplacesPreConstraintOrphan(t *testing.T) {
	pool := startPostgres(t)
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

func TestJobActivityStateTracksReturnedMetadata(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	for _, tc := range []struct{ column, value string }{
		{"company_slug", "'new-company'"},
		{"posted_at", "'2026-01-01T00:00:00Z'"},
		{"created_at", "'2026-01-01T00:00:00Z'"},
	} {
		t.Run(tc.column, func(t *testing.T) {
			id := insertJob(t, pool, "activity-metadata-"+tc.column)
			for attempt := range 2 {
				if _, err := pool.Exec(ctx, `UPDATE jobs SET `+tc.column+`=`+tc.value+` WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
				var version int64
				if err := pool.QueryRow(ctx, `SELECT state_version FROM job_activity WHERE job_id=$1`, id).Scan(&version); err != nil {
					t.Fatal(err)
				}
				if version != 1 {
					t.Fatalf("metadata write %d: version=%d, want 1", attempt+1, version)
				}
			}
		})
	}
}
