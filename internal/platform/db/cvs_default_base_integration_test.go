//go:build integration

// Integration tests for the default-base rule (cvs.sql: GetBaseCVByUser / ListBaseCVsByUser)
// and migration 0231's base_cv_id. SQL behaviour, so it needs a real Postgres.
// Requires Docker (testcontainers).
package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func cvUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func newBase(t *testing.T, q *Queries, userID int64, title string) uuid.UUID {
	t.Helper()
	row, err := q.CreateCV(context.Background(), CreateCVParams{
		UserID: userID, Title: title, TemplateID: "classic-ats", Data: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("create base %q: %v", title, err)
	}
	return row.ID
}

func tailorFrom(t *testing.T, pool *pgxpool.Pool, q *Queries, userID int64, base uuid.UUID, slug string) uuid.UUID {
	t.Helper()
	jobID := insertJob(t, pool, slug)
	row, err := q.CreateTailoredCV(context.Background(), CreateTailoredCVParams{
		UserID: userID, Title: "T " + slug, TemplateID: "classic-ats", Data: []byte(`{}`),
		JobID:    pgtype.Int8{Int64: jobID, Valid: true},
		BaseCvID: pgtype.UUID{Bytes: base, Valid: base != uuid.Nil},
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	return row.ID
}

func defaultBaseID(t *testing.T, q *Queries, userID int64) uuid.UUID {
	t.Helper()
	row, err := q.GetBaseCVByUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("default base: %v", err)
	}
	return row.ID
}

func TestDefaultBaseIsOldestCreatedAndEditsDoNotMoveIt(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	u := cvUser(t, pool, "a@example.test")

	first := newBase(t, q, u, "first")
	second := newBase(t, q, u, "second")

	if got := defaultBaseID(t, q, u); got != first {
		t.Fatalf("default = %s, want oldest-created %s", got, first)
	}
	// Editing the other base (newest updated_at) must not take the default.
	if _, err := q.UpdateCV(ctx, UpdateCVParams{ID: second, UserID: u, Title: "second!", TemplateID: "classic-ats", Data: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if got := defaultBaseID(t, q, u); got != first {
		t.Fatalf("after editing the other base default = %s, want %s", got, first)
	}
}

func TestDefaultBaseFollowsTheNewestTailoredCopyAndFallsBackWhenItsBaseIsDeleted(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	u := cvUser(t, pool, "b@example.test")

	first := newBase(t, q, u, "first")
	second := newBase(t, q, u, "second")
	older := tailorFrom(t, pool, q, u, first, "j1")
	tailorFrom(t, pool, q, u, second, "j2")

	if got := defaultBaseID(t, q, u); got != second {
		t.Fatalf("default = %s, want the base of the newest tailored copy %s", got, second)
	}
	items, err := q.ListBaseCVsByUser(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != first || items[1].ID != second {
		t.Fatalf("list order = %+v, want oldest-created first", items)
	}
	if items[0].IsDefault || !items[1].IsDefault {
		t.Fatalf("is_default = %v/%v, want false/true", items[0].IsDefault, items[1].IsDefault)
	}

	// Deleting the base the newest copy came from: the copy survives with NULL base_cv_id and
	// the default falls back to the oldest base.
	if _, err := q.DeleteCV(ctx, DeleteCVParams{ID: second, UserID: u}); err != nil {
		t.Fatal(err)
	}
	if got := defaultBaseID(t, q, u); got != first {
		t.Fatalf("after deleting the default base: default = %s, want %s", got, first)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cvs WHERE user_id = $1 AND is_tailored AND base_cv_id IS NULL`, u).Scan(&n); err != nil || n != 1 {
		t.Fatalf("tailored copies with NULL base after delete = %d (err %v), want 1", n, err)
	}
	_ = older
}

func TestDuplicateBaseCVCopiesDocumentAndFlagAndRefusesTailored(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	u := cvUser(t, pool, "c@example.test")
	other := cvUser(t, pool, "d@example.test")

	base := newBase(t, q, u, "orig")
	if _, err := q.MarkCVHandEdited(ctx, MarkCVHandEditedParams{ID: base, UserID: u}); err != nil {
		t.Fatal(err)
	}
	dup, err := q.DuplicateBaseCV(ctx, DuplicateBaseCVParams{ID: base, UserID: u, Title: "orig (copy)"})
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	got, err := q.GetCVByID(ctx, GetCVByIDParams{ID: dup.ID, UserID: u})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "orig (copy)" || got.IsTailored || !got.HandEdited {
		t.Fatalf("duplicate = %+v, want untailored hand_edited copy titled 'orig (copy)'", got)
	}
	if defaultBaseID(t, q, u) != base {
		t.Fatal("duplicating moved the default")
	}

	tailored := tailorFrom(t, pool, q, u, base, "j9")
	if _, err := q.DuplicateBaseCV(ctx, DuplicateBaseCVParams{ID: tailored, UserID: u, Title: "x"}); err == nil {
		t.Fatal("duplicating a tailored CV must return no row")
	}
	if _, err := q.DuplicateBaseCV(ctx, DuplicateBaseCVParams{ID: base, UserID: other, Title: "x"}); err == nil {
		t.Fatal("duplicating a foreign CV must return no row")
	}
}
