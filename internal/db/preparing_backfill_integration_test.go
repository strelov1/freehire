//go:build integration

// Integration test for the one-time preparing-stage backfill: applications the shipped
// EnsureOnBoard wrote as stage='applied' with no applied_at, before it was changed to write
// stage='preparing' directly (see internal/handler/cv.go). Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestBackfillPreparingStage(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "backfill@example.test")

	// Tailored: stage='applied', no applied_at, AND a tailored CV on record — the shape the
	// old EnsureOnBoard wrote, and the one the backfill must correct.
	tailoredJob := insertJob(t, pool, "backfill-tailored")
	if _, err := q.TrackJob(ctx, TrackJobParams{
		UserID: uid, JobID: tailoredJob, Stage: pgtype.Text{String: "applied", Valid: true},
	}); err != nil {
		t.Fatalf("seed tailored TrackJob: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO cvs (user_id, title, template_id, data, job_id, is_tailored)
		 VALUES ($1, 'Tailored', 'classic-ats', '{}'::jsonb, $2, true)`, uid, tailoredJob); err != nil {
		t.Fatalf("seed cv: %v", err)
	}

	// Manually dragged: identical stage/applied_at shape, but NO tailored CV — a candidate's
	// own undated "Applied" drag. The backfill must leave this alone.
	draggedJob := insertJob(t, pool, "backfill-dragged")
	if _, err := q.TrackJob(ctx, TrackJobParams{
		UserID: uid, JobID: draggedJob, Stage: pgtype.Text{String: "applied", Valid: true},
	}); err != nil {
		t.Fatalf("seed dragged TrackJob: %v", err)
	}

	// Untouched control: already promoted past applied, still with a tailored CV — proves the
	// backfill's WHERE clause, not just the EXISTS join, is what protects advanced stages.
	advancedJob := insertJob(t, pool, "backfill-advanced")
	if _, err := q.TrackJob(ctx, TrackJobParams{
		UserID: uid, JobID: advancedJob, Stage: pgtype.Text{String: "interview", Valid: true},
	}); err != nil {
		t.Fatalf("seed advanced TrackJob: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO cvs (user_id, title, template_id, data, job_id, is_tailored)
		 VALUES ($1, 'Tailored', 'classic-ats', '{}'::jsonb, $2, true)`, uid, advancedJob); err != nil {
		t.Fatalf("seed cv: %v", err)
	}

	count, err := q.BackfillPreparingStage(ctx)
	if err != nil {
		t.Fatalf("BackfillPreparingStage: %v", err)
	}
	if count != 1 {
		t.Errorf("backfilled = %d, want 1 (only the tailored+applied+undated row)", count)
	}

	assertStage := func(jobID int64, want string) {
		var got string
		if err := pool.QueryRow(ctx,
			`SELECT stage FROM applications WHERE user_id = $1 AND job_id = $2`, uid, jobID).Scan(&got); err != nil {
			t.Fatalf("read stage for job %d: %v", jobID, err)
		}
		if got != want {
			t.Errorf("job %d stage = %q, want %q", jobID, got, want)
		}
	}
	assertStage(tailoredJob, "preparing")
	assertStage(draggedJob, "applied")
	assertStage(advancedJob, "interview")

	// The correction is itself a recorded stage_set event, source=system: the platform
	// corrected its own past write, not the candidate.
	var source, signal string
	if err := pool.QueryRow(ctx,
		`SELECT source, signal FROM application_events
		  WHERE user_id = $1 AND job_id = $2 AND kind = 'stage_set'
		  ORDER BY id DESC LIMIT 1`, uid, tailoredJob).Scan(&source, &signal); err != nil {
		t.Fatalf("read backfill ledger event: %v", err)
	}
	if source != "system" {
		t.Errorf("backfill event source = %q, want system", source)
	}
	if signal != "preparing" {
		t.Errorf("backfill event signal = %q, want preparing", signal)
	}

	// Idempotent: a second run matches nothing, because the row it just corrected no longer
	// reads stage='applied'.
	count, err = q.BackfillPreparingStage(ctx)
	if err != nil {
		t.Fatalf("second BackfillPreparingStage: %v", err)
	}
	if count != 0 {
		t.Errorf("second run backfilled = %d, want 0", count)
	}
}
