//go:build integration

// Integration tests for TouchAssistantSession and SetAssistantSessionLabel's owner
// scoping. Every other write on assistant_sessions (Delete, and every read) already
// requires id AND user_id to match; these two took a bare id, so a caller with a
// session id it does not own could still touch or relabel it — a real Postgres is the
// only way to prove the WHERE clause change actually blocks a foreign write. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTouchAssistantSessionIsOwnerScoped(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	owner := seedAssistantUser(t, pool, "touch-owner@example.test")
	stranger := seedAssistantUser(t, pool, "touch-stranger@example.test")

	sess, err := q.CreateAssistantSession(ctx, CreateAssistantSessionParams{UserID: owner, Preset: "chat"})
	if err != nil {
		t.Fatalf("CreateAssistantSession: %v", err)
	}

	// Pin updated_at to something clearly in the past so a real touch is detectable.
	past := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE assistant_sessions SET updated_at = $1 WHERE id = $2`, past, sess.ID); err != nil {
		t.Fatalf("pin updated_at: %v", err)
	}

	if err := q.TouchAssistantSession(ctx, TouchAssistantSessionParams{ID: sess.ID, UserID: stranger}); err != nil {
		t.Fatalf("TouchAssistantSession(stranger): %v", err)
	}
	if got := updatedAtOf(ctx, t, pool, sess.ID); !got.Equal(past) {
		t.Errorf("updated_at = %v, want unchanged at %v — a non-owner's touch must affect nothing", got, past)
	}

	if err := q.TouchAssistantSession(ctx, TouchAssistantSessionParams{ID: sess.ID, UserID: owner}); err != nil {
		t.Fatalf("TouchAssistantSession(owner): %v", err)
	}
	if got := updatedAtOf(ctx, t, pool, sess.ID); got.Equal(past) {
		t.Error("updated_at unchanged after the real owner's touch — want it bumped")
	}
}

func TestSetAssistantSessionLabelIsOwnerScoped(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	owner := seedAssistantUser(t, pool, "label-owner@example.test")
	stranger := seedAssistantUser(t, pool, "label-stranger@example.test")

	sess, err := q.CreateAssistantSession(ctx, CreateAssistantSessionParams{UserID: owner, Preset: "chat"})
	if err != nil {
		t.Fatalf("CreateAssistantSession: %v", err)
	}

	err = q.SetAssistantSessionLabel(ctx, SetAssistantSessionLabelParams{
		ID: sess.ID, UserID: stranger, Label: pgtype.Text{String: "renamed by a stranger", Valid: true},
	})
	if err != nil {
		t.Fatalf("SetAssistantSessionLabel(stranger): %v", err)
	}
	if got := labelOf(ctx, t, pool, sess.ID); got.Valid {
		t.Errorf("label = %+v, want still unset — a non-owner's relabel must affect nothing", got)
	}

	err = q.SetAssistantSessionLabel(ctx, SetAssistantSessionLabelParams{
		ID: sess.ID, UserID: owner, Label: pgtype.Text{String: "find go jobs", Valid: true},
	})
	if err != nil {
		t.Fatalf("SetAssistantSessionLabel(owner): %v", err)
	}
	if got := labelOf(ctx, t, pool, sess.ID); !got.Valid || got.String != "find go jobs" {
		t.Errorf("label = %+v, want %q from the real owner", got, "find go jobs")
	}
}

func updatedAtOf(ctx context.Context, t *testing.T, pool *pgxpool.Pool, id uuid.UUID) time.Time {
	t.Helper()
	var v time.Time
	if err := pool.QueryRow(ctx, `SELECT updated_at FROM assistant_sessions WHERE id = $1`, id).Scan(&v); err != nil {
		t.Fatalf("query updated_at for session %s: %v", id, err)
	}
	return v
}

func labelOf(ctx context.Context, t *testing.T, pool *pgxpool.Pool, id uuid.UUID) pgtype.Text {
	t.Helper()
	var v pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT label FROM assistant_sessions WHERE id = $1`, id).Scan(&v); err != nil {
		t.Fatalf("query label for session %s: %v", id, err)
	}
	return v
}
