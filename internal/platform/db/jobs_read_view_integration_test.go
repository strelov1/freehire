//go:build integration

package db

import (
	"context"
	"slices"
	"testing"
	"time"
)

// activityColumns are the three stamps the activity split moved out of jobs. jobs_read serves
// them from job_activity under the same names, so db.Job (the view's row) keeps its shape.
var activityColumns = []string{"last_seen_at", "hydrated_at", "liveness_strikes"}

// TestJobsReadCoversEveryJobsColumn pins the one way the view can drift: a view's column list
// is fixed when it is created, so a later `ALTER TABLE jobs ADD COLUMN` would be missing from
// db.Job (and from every reader of it) until the view is replaced. The compiler catches a
// reader that asks for the new column; this catches the column nobody has read yet.
func TestJobsReadCoversEveryJobsColumn(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	columns := func(rel string) []string {
		rows, err := pool.Query(ctx, `SELECT column_name FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 ORDER BY column_name`, rel)
		if err != nil {
			t.Fatalf("columns of %s: %v", rel, err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Fatal(err)
			}
			out = append(out, c)
		}
		return out
	}

	view := columns("jobs_read")
	if len(view) == 0 {
		t.Fatal("jobs_read has no columns: the view is missing")
	}
	// The contract release drops the three legacy columns from jobs; until then the base table
	// still has them, and the view must not select them from there.
	var want []string
	for _, c := range columns("jobs") {
		if !slices.Contains(activityColumns, c) {
			want = append(want, c)
		}
	}
	want = append(want, activityColumns...)
	slices.Sort(want)
	if !slices.Equal(view, want) {
		t.Errorf("jobs_read columns differ from jobs minus the legacy stamps plus job_activity's.\n view: %v\n want: %v\n"+
			"A column added to jobs needs the view replaced in the same migration.", view, want)
	}
}

// The three stamps read through the view come from job_activity, never from the legacy columns
// of the base table, and a job without an activity row is still served: LEFT JOIN, so the hole
// reads as "never seen" (last_seen_at NULL, which search.Unseen treats as no evidence) instead
// of hiding the job.
func TestJobsReadServesTheActivityStampsAndFailsOpen(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:view", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	seen := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	hydrated := seen.Add(-48 * time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = $2, hydrated_at = $3, liveness_strikes = 2 WHERE job_id = $1`,
		job.ID, seen, hydrated); err != nil {
		t.Fatalf("set activity: %v", err)
	}
	// A legacy column the view must NOT read: made visibly different from the activity row.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET last_seen_at = $2, hydrated_at = $2, liveness_strikes = 9 WHERE id = $1`,
		job.ID, seen.Add(-1000*time.Hour)); err != nil {
		t.Fatalf("set legacy columns: %v", err)
	}
	// The legacy-column write above is bridged into activity, so state the activity row last.
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = $2, hydrated_at = $3, liveness_strikes = 2 WHERE job_id = $1`,
		job.ID, seen, hydrated); err != nil {
		t.Fatalf("restate activity: %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if !got.LastSeenAt.Time.Equal(seen) || !got.HydratedAt.Time.Equal(hydrated) || got.LivenessStrikes != 2 {
		t.Errorf("stamps through the view = %v / %v / %d, want the activity row's %v / %v / 2",
			got.LastSeenAt.Time, got.HydratedAt.Time, got.LivenessStrikes, seen, hydrated)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM job_activity WHERE job_id = $1`, job.ID); err != nil {
		t.Fatalf("drop activity row: %v", err)
	}
	orphan, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("a job without an activity row must still be served: %v", err)
	}
	if orphan.LastSeenAt.Valid || orphan.HydratedAt.Valid || orphan.LivenessStrikes != 0 {
		t.Errorf("job without activity reads %v / %v / %d, want NULL / NULL / 0",
			orphan.LastSeenAt, orphan.HydratedAt, orphan.LivenessStrikes)
	}
}
