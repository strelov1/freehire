//go:build integration

// Integration test for QueriesRepository.UpsertIfUnchanged — the fix for MergeSkills'
// read-then-blind-upsert race: the guarded write must land when updated_at still
// matches what was read, and must report ErrConflict (no row) when it doesn't. Only a
// real Postgres can verify the SQL's WHERE guard actually behaves this way. Run with:
// go test -tags=integration ./internal/userprofile/
package userprofile_test

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/testdb"
	"github.com/strelov1/freehire/internal/userprofile"
)

func TestUpsertIfUnchanged_GuardsOnUpdatedAt(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE user_profiles, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, "profile-race@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	repo := userprofile.NewQueriesRepository(db.New(pool))
	created, err := repo.Upsert(ctx, userID, []string{"backend"}, []string{"go"}, []string{}, nil)
	if err != nil {
		t.Fatalf("seed Upsert: %v", err)
	}
	if created.UpdatedAt == nil {
		t.Fatal("seeded profile has no updated_at")
	}

	// A guarded write against the just-read updated_at must land.
	updated, err := repo.UpsertIfUnchanged(ctx, userID, []string{"backend"}, []string{"go", "docker"}, []string{}, nil, *created.UpdatedAt)
	if err != nil {
		t.Fatalf("UpsertIfUnchanged against a matching updated_at: %v", err)
	}
	if len(updated.Skills) != 2 {
		t.Errorf("Skills = %v, want [go docker]", updated.Skills)
	}

	// A second guarded write against the now-STALE updated_at (the row moved on the
	// write above) must be rejected as a conflict, not silently applied.
	_, err = repo.UpsertIfUnchanged(ctx, userID, []string{"backend"}, []string{"go", "docker", "kubernetes"}, []string{}, nil, *created.UpdatedAt)
	if !errors.Is(err, userprofile.ErrConflict) {
		t.Errorf("UpsertIfUnchanged against a stale updated_at: err=%v, want ErrConflict", err)
	}

	// The rejected write must not have landed.
	current, err := repo.Get(ctx, userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(current.Skills) != 2 {
		t.Errorf("Skills after rejected conflicting write = %v, want unchanged [go docker]", current.Skills)
	}
}
