//go:build integration

// Integration tests for what makes a CV the BASE CV (see the explicit-base-cv change). The base
// used to be inferred from `job_id IS NULL`, an absence that cmd/prune manufactures: deleting a
// vacancy nulls its tailored copy's link and the orphan — freshly edited — outranked the real base.
// "The base" is still derived (the newest non-tailored CV, of which a user may own several); what
// changed is that a tailored copy stays tailored after its vacancy is gone.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedCVJob inserts a vacancy a tailored CV can bind to.
func seedCVJob(t *testing.T, pool *pgxpool.Pool, slug string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, public_slug)
		 VALUES ('test', $1, 'https://e.test/'||$1, 'Backend Engineer', $1) RETURNING id`,
		slug).Scan(&id); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return id
}

// TestBaseCVSurvivesAPrunedVacancy is the regression the change exists for: pruning a vacancy must
// not promote its tailored copy to base. The orphan is edited last on purpose — under the old
// `job_id IS NULL ORDER BY updated_at DESC` rule that is exactly what made it win.
func TestBaseCVSurvivesAPrunedVacancy(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()

	owner := seedCVUser(t, pool, "pruned-vacancy@example.com")
	base, err := q.CreateCV(ctx, CreateCVParams{
		UserID: owner, Title: "Base", TemplateID: "classic-ats",
		Data: []byte(`{"summary":"the real base"}`),
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}

	jobID := seedCVJob(t, pool, "backend-to-be-pruned")
	tailored, err := q.CreateTailoredCV(ctx, CreateTailoredCVParams{
		UserID: owner, Title: "Tailored", TemplateID: "classic-ats",
		Data:  []byte(`{"summary":"tailored for a vacancy that will vanish"}`),
		JobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	// Touch the tailored copy last, so it is the newest vacancy-less row once the job goes.
	if _, err := q.UpdateCV(ctx, UpdateCVParams{
		ID: tailored.ID, UserID: owner, Title: "Tailored", TemplateID: "classic-ats",
		Data: []byte(`{"summary":"edited most recently"}`),
	}); err != nil {
		t.Fatalf("touch tailored: %v", err)
	}

	// cmd/prune hard-deletes the vacancy; the FK nulls the tailored copy's link.
	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, jobID); err != nil {
		t.Fatalf("prune job: %v", err)
	}

	got, err := q.GetBaseCVByUser(ctx, owner)
	if err != nil {
		t.Fatalf("base lookup after prune: %v", err)
	}
	if got.ID != base.ID {
		t.Errorf("base lookup returned %q (%s), want the real base %q — a pruned vacancy must not promote its tailored copy",
			got.Title, got.ID, base.Title)
	}
}

// TestBaseCVAbsentWhenOnlyAnOrphanRemains pins the other half: an orphaned tailored copy is not a
// base, so a user who has only one has NO base CV and must get a fresh one seeded.
func TestBaseCVAbsentWhenOnlyAnOrphanRemains(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()

	owner := seedCVUser(t, pool, "orphan-only@example.com")
	jobID := seedCVJob(t, pool, "only-vacancy-pruned")
	if _, err := q.CreateTailoredCV(ctx, CreateTailoredCVParams{
		UserID: owner, Title: "Tailored", TemplateID: "classic-ats",
		Data:  []byte(`{"summary":"the only cv"}`),
		JobID: pgtype.Int8{Int64: jobID, Valid: true},
	}); err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, jobID); err != nil {
		t.Fatalf("prune job: %v", err)
	}

	if _, err := q.GetBaseCVByUser(ctx, owner); err == nil {
		t.Error("base lookup found a base CV; want none — the user's only CV is an orphaned tailored copy")
	}
}
