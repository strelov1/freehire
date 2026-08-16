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

// The rail is the website's session list — a browsing conversation only works over the
// extension's own connection, so listing it there would offer a chat that silently loses
// its one distinguishing tool the moment it is opened. See the confine-browse-preset-to-
// extension change.
func TestListAssistantChatSessionsExcludesTailorAndBrowse(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedAssistantUser(t, pool, "rail-scope@example.test")
	jobID := insertJob(t, pool, "rail-scope-vacancy")
	cv, err := q.CreateCV(ctx, CreateCVParams{
		UserID: user, Title: "Rail scope", TemplateID: "classic-ats", Data: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("create cv: %v", err)
	}

	for _, preset := range []string{"chat", "profile", "browse", "interview", "debrief", "tailor"} {
		params := CreateAssistantSessionParams{UserID: user, Preset: preset}
		switch preset {
		case "interview", "debrief":
			params.JobID = &jobID
		case "tailor":
			params.CvID = &cv.ID
			params.JobID = &jobID
		}
		if _, err := q.CreateAssistantSession(ctx, params); err != nil {
			t.Fatalf("create %s session: %v", preset, err)
		}
	}

	rows, err := q.ListAssistantChatSessions(ctx, user)
	if err != nil {
		t.Fatalf("ListAssistantChatSessions: %v", err)
	}
	got := make(map[string]bool, len(rows))
	for _, r := range rows {
		got[r.Preset] = true
	}
	for _, want := range []string{"chat", "profile", "interview", "debrief"} {
		if !got[want] {
			t.Errorf("rail is missing the %q session; got %v", want, got)
		}
	}
	for _, excluded := range []string{"tailor", "browse"} {
		if got[excluded] {
			t.Errorf("rail lists a %q session, want it excluded", excluded)
		}
	}
}
