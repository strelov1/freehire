//go:build integration

// Integration tests for the preset vocabulary pinned on assistant_sessions. The
// constraint is the reason a new preset is a schema change and not just a Go constant,
// and only a real Postgres can answer whether it admits one. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedAssistantUser creates an account a session can belong to.
func seedAssistantUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

// A rehearsal session records the 'interview' preset, so the CHECK has to admit it —
// otherwise every rehearsal fails at creation with a 23514.
func TestAssistantSessionsAdmitTheInterviewPreset(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedAssistantUser(t, pool, "rehearsal@example.test")
	jobID := insertJob(t, pool, "rehearsal-vacancy")

	sess, err := q.CreateAssistantSession(ctx, CreateAssistantSessionParams{
		UserID: user, Preset: "interview", JobID: &jobID,
	})
	if err != nil {
		t.Fatalf("CreateAssistantSession(interview): %v", err)
	}
	if sess.Preset != "interview" {
		t.Errorf("preset = %q, want %q", sess.Preset, "interview")
	}
	if sess.JobID == nil || *sess.JobID != jobID {
		t.Errorf("job_id = %v, want %d — a rehearsal is bound to its vacancy", sess.JobID, jobID)
	}
	if sess.CvID != nil {
		t.Errorf("cv_id = %v, want nil — a rehearsal edits no CV", sess.CvID)
	}
}

// Widening the vocabulary must not unpin it. Each migration rewrites the CHECK in full
// because Postgres cannot alter a CHECK's expression, and a rewrite that dropped the
// constraint instead of replacing it would leave the column accepting anything — which
// is exactly the state that lets a session record a preset nothing implements.
func TestAssistantSessionsStillRejectAnUnknownPreset(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedAssistantUser(t, pool, "unknown-preset@example.test")

	_, err := q.CreateAssistantSession(ctx, CreateAssistantSessionParams{
		UserID: user, Preset: "negotiation",
	})
	if err == nil {
		t.Fatal("CreateAssistantSession accepted an unimplemented preset; the CHECK is gone")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("error = %v, want a 23514 check violation", err)
	}
}
