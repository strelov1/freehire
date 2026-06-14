//go:build integration

// Integration tests for the application-stage SQL semantics — TrackJob's partial
// (COALESCE) upsert and MarkJobApplied seeding stage='applied' only when unset —
// which are SQL behavior and can only be verified against a real Postgres. Run
// with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestClearJobProgress(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "clearprogress@example.test")
	jid := insertJob(t, pool, "clear-1")

	// Set up the full pipeline state: saved, applied, stage=interview, notes set.
	if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("MarkJobApplied: %v", err)
	}
	if _, err := q.TrackJob(ctx, TrackJobParams{
		UserID: uid, JobID: jid,
		Stage: pgtype.Text{String: "interview", Valid: true},
		Notes: pgtype.Text{String: "great team", Valid: true},
	}); err != nil {
		t.Fatalf("TrackJob: %v", err)
	}

	row, err := q.ClearJobProgress(ctx, ClearJobProgressParams{UserID: uid, JobID: jid})
	if err != nil {
		t.Fatalf("ClearJobProgress: %v", err)
	}
	if row.Stage.Valid {
		t.Errorf("stage = %q, want NULL", row.Stage.String)
	}
	if row.AppliedAt.Valid {
		t.Errorf("applied_at = %v, want NULL", row.AppliedAt.Time)
	}
	if !row.SavedAt.Valid {
		t.Error("saved_at was cleared, want it kept")
	}
	if !row.Notes.Valid || row.Notes.String != "great team" {
		t.Errorf("notes = %q (valid=%v), want %q kept", row.Notes.String, row.Notes.Valid, "great team")
	}
}

func TestUntrackJob(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "untrack@example.test")
	jid := insertJob(t, pool, "untrack-1")

	// Set up the full state: view, save, apply, track.
	if _, err := q.RecordJobView(ctx, RecordJobViewParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("RecordJobView: %v", err)
	}
	if _, err := q.SaveJob(ctx, SaveJobParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}
	if _, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("MarkJobApplied: %v", err)
	}
	if _, err := q.TrackJob(ctx, TrackJobParams{
		UserID: uid, JobID: jid,
		Stage: pgtype.Text{String: "offer", Valid: true},
		Notes: pgtype.Text{String: "great team", Valid: true},
	}); err != nil {
		t.Fatalf("TrackJob: %v", err)
	}

	row, err := q.UntrackJob(ctx, UntrackJobParams{UserID: uid, JobID: jid})
	if err != nil {
		t.Fatalf("UntrackJob: %v", err)
	}
	if row.SavedAt.Valid {
		t.Errorf("saved_at = %v, want NULL", row.SavedAt.Time)
	}
	if row.AppliedAt.Valid {
		t.Errorf("applied_at = %v, want NULL", row.AppliedAt.Time)
	}
	if row.Stage.Valid {
		t.Errorf("stage = %q, want NULL", row.Stage.String)
	}
	if row.Notes.Valid {
		t.Errorf("notes = %q, want NULL", row.Notes.String)
	}
	if !row.ViewedAt.Valid {
		t.Error("viewed_at was cleared, want it kept")
	}
}

func TestTrackJobAndStageSeeding(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "tracker@example.test")
	jid := insertJob(t, pool, "track-1")

	t.Run("track stage-only creates the row and leaves notes null", func(t *testing.T) {
		row, err := q.TrackJob(ctx, TrackJobParams{
			UserID: uid, JobID: jid, Stage: pgtype.Text{String: "interview", Valid: true},
		})
		if err != nil {
			t.Fatalf("TrackJob: %v", err)
		}
		if row.Stage.String != "interview" {
			t.Errorf("stage = %q, want interview", row.Stage.String)
		}
		if row.Notes.Valid {
			t.Errorf("notes should be null, got %q", row.Notes.String)
		}
	})

	t.Run("track notes-only leaves the stage unchanged (partial update)", func(t *testing.T) {
		row, err := q.TrackJob(ctx, TrackJobParams{
			UserID: uid, JobID: jid, Notes: pgtype.Text{String: "recruiter called back", Valid: true},
		})
		if err != nil {
			t.Fatalf("TrackJob: %v", err)
		}
		if row.Stage.String != "interview" {
			t.Errorf("stage changed to %q, want it left at interview", row.Stage.String)
		}
		if row.Notes.String != "recruiter called back" {
			t.Errorf("notes = %q", row.Notes.String)
		}
	})

	t.Run("MarkApplied seeds stage='applied' only when unset", func(t *testing.T) {
		jid2 := insertJob(t, pool, "track-2")

		row, err := q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid2})
		if err != nil {
			t.Fatalf("MarkJobApplied: %v", err)
		}
		if row.Stage.String != "applied" {
			t.Errorf("first apply: stage = %q, want applied", row.Stage.String)
		}

		// Advance the stage, then re-apply: the advanced stage must survive.
		if _, err := q.TrackJob(ctx, TrackJobParams{
			UserID: uid, JobID: jid2, Stage: pgtype.Text{String: "offer", Valid: true},
		}); err != nil {
			t.Fatalf("TrackJob(offer): %v", err)
		}
		row, err = q.MarkJobApplied(ctx, MarkJobAppliedParams{UserID: uid, JobID: jid2})
		if err != nil {
			t.Fatalf("re-apply: %v", err)
		}
		if row.Stage.String != "offer" {
			t.Errorf("re-apply clobbered stage to %q, want offer", row.Stage.String)
		}
	})
}
