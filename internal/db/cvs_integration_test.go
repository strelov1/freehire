//go:build integration

// Integration tests for the cvs queries (add-cv-builder): CRUD round-trip and owner
// isolation — a foreign or missing id must return no row (Get/Update) or a zero delete
// count, which the cv.Store maps to ErrNotFound / 404.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func truncateCVs(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"TRUNCATE cvs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate cvs: %v", err)
	}
}

func seedCVUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func TestCVsCRUDRoundTrip(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()

	user := seedCVUser(t, pool, "cv-owner@example.com")
	blob := []byte(`{"header":{"full_name":"Ada"},"summary":"eng"}`)

	created, err := q.CreateCV(ctx, CreateCVParams{UserID: user, Title: "General", TemplateID: "classic-ats", Data: blob})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Title != "General" || created.TemplateID != "classic-ats" {
		t.Errorf("create returned wrong metadata: %+v", created)
	}

	got, err := q.GetCVByID(ctx, GetCVByIDParams{ID: created.ID, UserID: user})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// jsonb is stored normalized (re-spaced, key-reordered), so compare semantically
	// rather than by raw bytes.
	var stored map[string]any
	if err := json.Unmarshal(got.Data, &stored); err != nil {
		t.Fatalf("stored data not valid json: %v", err)
	}
	if header, _ := stored["header"].(map[string]any); header["full_name"] != "Ada" {
		t.Errorf("data not round-tripped: %s", got.Data)
	}

	list, err := q.ListCVsByUser(ctx, user)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: err=%v len=%d", err, len(list))
	}

	updated, err := q.UpdateCV(ctx, UpdateCVParams{ID: created.ID, UserID: user, Title: "Tailored", TemplateID: "classic-ats", Data: []byte(`{"summary":"x"}`)})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "Tailored" {
		t.Errorf("update title = %q, want Tailored", updated.Title)
	}

	n, err := q.DeleteCV(ctx, DeleteCVParams{ID: created.ID, UserID: user})
	if err != nil || n != 1 {
		t.Fatalf("delete: err=%v n=%d", err, n)
	}
	if _, err := q.GetCVByID(ctx, GetCVByIDParams{ID: created.ID, UserID: user}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("post-delete get err = %v, want ErrNoRows", err)
	}
}

func TestCVsOwnerIsolation(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()

	owner := seedCVUser(t, pool, "owner@example.com")
	other := seedCVUser(t, pool, "other@example.com")

	created, err := q.CreateCV(ctx, CreateCVParams{UserID: owner, Title: "Mine", TemplateID: "classic-ats", Data: []byte(`{}`)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := q.GetCVByID(ctx, GetCVByIDParams{ID: created.ID, UserID: other}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("foreign get err = %v, want ErrNoRows", err)
	}
	if _, err := q.UpdateCV(ctx, UpdateCVParams{ID: created.ID, UserID: other, Title: "Hijack", TemplateID: "classic-ats", Data: []byte(`{}`)}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("foreign update err = %v, want ErrNoRows", err)
	}
	if n, err := q.DeleteCV(ctx, DeleteCVParams{ID: created.ID, UserID: other}); err != nil || n != 0 {
		t.Errorf("foreign delete: err=%v n=%d, want n=0", err, n)
	}
	if list, err := q.ListCVsByUser(ctx, other); err != nil || len(list) != 0 {
		t.Errorf("foreign list: err=%v len=%d, want 0", err, len(list))
	}
}

// TestBaseCVAndTailoredCopy covers the two-tier tailoring queries: GetBaseCVByUser picks the
// user's newest non-tailored CV (and reports no row when there is none), and CreateTailoredCV
// binds a copy to a vacancy via job_id without that copy shadowing the base.
func TestBaseCVAndTailoredCopy(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()

	user := seedCVUser(t, pool, "tailor@example.com")

	// No base CV yet → no row (the caller then seeds one from the résumé).
	if _, err := q.GetBaseCVByUser(ctx, user); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("empty base err = %v, want ErrNoRows", err)
	}

	// Two non-tailored CVs: the base is the most-recently-created (id-tiebroken) one.
	if _, err := q.CreateCV(ctx, CreateCVParams{UserID: user, Title: "Older", TemplateID: "classic-ats", Data: []byte(`{"summary":"old"}`)}); err != nil {
		t.Fatalf("create older: %v", err)
	}
	newer, err := q.CreateCV(ctx, CreateCVParams{UserID: user, Title: "Newer", TemplateID: "classic-ats", Data: []byte(`{"summary":"new"}`)})
	if err != nil {
		t.Fatalf("create newer: %v", err)
	}
	base, err := q.GetBaseCVByUser(ctx, user)
	if err != nil {
		t.Fatalf("base: %v", err)
	}
	if base.ID != newer.ID {
		t.Errorf("base = %d, want newest %d", base.ID, newer.ID)
	}

	// A tailored copy bound to a vacancy is created with job_id and must NOT become the base.
	job := insertJob(t, pool, "job-ext-tailor")
	tailored, err := q.CreateTailoredCV(ctx, CreateTailoredCVParams{
		UserID: user, Title: "Tailored", TemplateID: "classic-ats",
		Data: base.Data, JobID: pgtype.Int8{Int64: job, Valid: true},
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	if again, err := q.GetBaseCVByUser(ctx, user); err != nil || again.ID != newer.ID {
		t.Errorf("base after tailoring = %d (err %v), want %d (tailored excluded)", again.ID, err, newer.ID)
	}
	if got, err := q.GetCVByID(ctx, GetCVByIDParams{ID: tailored.ID, UserID: user}); err != nil {
		t.Fatalf("get tailored: %v", err)
	} else if got.Title != "Tailored" {
		t.Errorf("tailored title = %q, want Tailored", got.Title)
	}
}
