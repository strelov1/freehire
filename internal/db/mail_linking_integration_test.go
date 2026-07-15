//go:build integration

// Integration tests for the cross-user isolation invariant of email→application
// linking: the classification worker must never link one user's email to another
// user's application, and the API reads/mutations must never cross the tenant
// boundary. These are SQL-scoping guarantees, so they can only be verified against
// a real Postgres. Run with: go test -tags=integration ./internal/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func applyToJob(t *testing.T, pool *pgxpool.Pool, userID, jobID int64, stage string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO user_jobs (user_id, job_id, applied_at, stage) VALUES ($1, $2, now(), $3)`,
		userID, jobID, stage)
	if err != nil {
		t.Fatalf("apply to job: %v", err)
	}
}

func insertLinkedEmail(t *testing.T, pool *pgxpool.Pool, userID int64, externalID string, jobID int64) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO emails (user_id, source, external_id, received_at, job_id)
		 VALUES ($1, 'gmail', $2, now(), $3) RETURNING id`,
		userID, externalID, jobID).Scan(&id)
	if err != nil {
		t.Fatalf("insert email: %v", err)
	}
	return id
}

// TestMailLinkingUserIsolation asserts the tenant boundary holds across every
// query the feature added: candidate pools, email reads, and mutations are all
// scoped to the caller, so user A can never touch or see user B's data.
func TestMailLinkingUserIsolation(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	userA := insertUser(t, pool, "a@example.test")
	userB := insertUser(t, pool, "b@example.test")
	// A shared public job both users applied to, plus a private-to-each job.
	shared := insertJob(t, pool, "shared")
	jobA := insertJob(t, pool, "job-a")
	jobB := insertJob(t, pool, "job-b")
	applyToJob(t, pool, userA, shared, "applied")
	applyToJob(t, pool, userB, shared, "applied")
	applyToJob(t, pool, userA, jobA, "applied")
	applyToJob(t, pool, userB, jobB, "applied")

	t.Run("candidate pool excludes the other user's applications", func(t *testing.T) {
		apps, err := q.ListUserApplicationsForMatch(ctx, userA)
		if err != nil {
			t.Fatalf("list apps: %v", err)
		}
		ids := map[int64]bool{}
		for _, a := range apps {
			ids[a.ID] = true
		}
		if !ids[shared] || !ids[jobA] {
			t.Fatalf("A's pool = %v, want it to include shared(%d) and jobA(%d)", ids, shared, jobA)
		}
		if ids[jobB] {
			t.Fatalf("A's candidate pool leaked B's application jobB(%d) — the agent could mis-link across users", jobB)
		}
	})

	// Both users have an email linked to the SAME shared job.
	emailA := insertLinkedEmail(t, pool, userA, "mail-a", shared)
	emailB := insertLinkedEmail(t, pool, userB, "mail-b", shared)

	t.Run("linked-email read is scoped even on a shared job", func(t *testing.T) {
		rows, err := q.ListJobEmails(ctx, ListJobEmailsParams{UserID: userA, JobID: pgtype.Int8{Int64: shared, Valid: true}})
		if err != nil {
			t.Fatalf("list job emails: %v", err)
		}
		if len(rows) != 1 || rows[0].ID != emailA {
			t.Fatalf("A sees %d emails on the shared job (ids %v), want only emailA(%d)", len(rows), rows, emailA)
		}
	})

	t.Run("GetUserApplication 404s for a job the caller does not track", func(t *testing.T) {
		if _, err := q.GetUserApplication(ctx, GetUserApplicationParams{UserID: userB, JobID: jobA}); err == nil {
			t.Fatalf("B fetched A's application jobA(%d) — must be not-found", jobA)
		}
	})

	t.Run("link mutations reject another user's email", func(t *testing.T) {
		// B applies A's email id to every mutation; each must match zero rows.
		if n, _ := q.LinkEmailToJob(ctx, LinkEmailToJobParams{ID: emailA, UserID: userB, JobID: pgtype.Int8{Int64: jobB, Valid: true}}); n != 0 {
			t.Errorf("B linked A's email: %d rows, want 0", n)
		}
		if n, _ := q.UnlinkEmail(ctx, UnlinkEmailParams{ID: emailA, UserID: userB}); n != 0 {
			t.Errorf("B unlinked A's email: %d rows, want 0", n)
		}
		if n, _ := q.ConfirmEmailLink(ctx, ConfirmEmailLinkParams{ID: emailA, UserID: userB}); n != 0 {
			t.Errorf("B confirmed A's email: %d rows, want 0", n)
		}
		// A's email is still linked to the shared job, untouched.
		var jobID pgtype.Int8
		if err := pool.QueryRow(ctx, `SELECT job_id FROM emails WHERE id=$1`, emailA).Scan(&jobID); err != nil {
			t.Fatalf("reread emailA: %v", err)
		}
		if !jobID.Valid || jobID.Int64 != shared {
			t.Fatalf("A's email link changed to %v after B's attempts, want shared(%d)", jobID, shared)
		}
	})

	t.Run("SetEmailClassification rejects another user's email", func(t *testing.T) {
		// B tries to stamp a classification on A's email; the user_id guard drops it.
		err := q.SetEmailClassification(ctx, SetEmailClassificationParams{
			StatusSignal: pgtype.Text{String: "rejection", Valid: true},
			Model:        pgtype.Text{String: "attacker", Valid: true},
			ID:           emailA,
			UserID:       userB,
		})
		if err != nil {
			t.Fatalf("set classification: %v", err)
		}
		var model pgtype.Text
		if err := pool.QueryRow(ctx, `SELECT classification_model FROM emails WHERE id=$1`, emailA).Scan(&model); err != nil {
			t.Fatalf("reread emailA: %v", err)
		}
		if model.Valid {
			t.Fatalf("B stamped A's email (model=%q) — the user_id write guard failed", model.String)
		}
		_ = emailB // present to prove two users' mail coexist without crossing
	})
}
