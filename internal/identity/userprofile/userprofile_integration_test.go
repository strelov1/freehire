//go:build integration

// Integration test for QueriesRepository.UpsertIfUnchanged — the fix for MergeSkills'
// read-then-blind-upsert race: the guarded write must land when updated_at still
// matches what was read, and must report ErrConflict (no row) when it doesn't. Only a
// real Postgres can verify the SQL's WHERE guard actually behaves this way. Run with:
// go test -tags=integration ./internal/identity/userprofile/
package userprofile_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestCreateIfAbsent_DoesNotReplaceConcurrentProfile(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, "profile-initial-race@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	svc := userprofile.New(userprofile.NewQueriesRepository(db.New(pool)))
	created, err := svc.CreateIfAbsent(ctx, userID, []string{"backend"}, []string{"go"},
		[]string{"senior"}, []string{"php"}, []string{"hh"}, []string{"acme"},
		&userprofile.LocationPreferences{WorkModes: []string{"remote"}}, "en")
	if err != nil {
		t.Fatalf("CreateIfAbsent: %v", err)
	}
	_, err = svc.CreateIfAbsent(ctx, userID, nil, []string{"python"}, nil, nil, nil, nil, nil)
	if !errors.Is(err, userprofile.ErrConflict) {
		t.Fatalf("second create err=%v, want ErrConflict", err)
	}
	stored, err := svc.Get(ctx, userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(stored, created) {
		t.Fatalf("conflicted creation replaced stored profile: got %+v, want %+v", stored, created)
	}
}

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
	created, err := repo.Upsert(ctx, userID, []string{"backend"}, []string{"go"}, []string{}, []string{}, []string{}, []string{}, []string{"en", "ru"}, nil)
	if err != nil {
		t.Fatalf("seed Upsert: %v", err)
	}
	if created.UpdatedAt == nil {
		t.Fatal("seeded profile has no updated_at")
	}

	// A guarded write against the just-read updated_at must land.
	updated, err := repo.UpsertIfUnchanged(ctx, userID, []string{"backend"}, []string{"go", "docker"}, []string{}, []string{}, []string{}, []string{}, []string{"en", "ru"}, nil, *created.UpdatedAt)
	if err != nil {
		t.Fatalf("UpsertIfUnchanged against a matching updated_at: %v", err)
	}
	if len(updated.JobLanguages) != 2 || updated.JobLanguages[0] != "en" || updated.JobLanguages[1] != "ru" {
		t.Fatalf("languages did not round-trip: %v", updated.JobLanguages)
	}
	if len(updated.Skills) != 2 {
		t.Errorf("Skills = %v, want [go docker]", updated.Skills)
	}

	// A second guarded write against the now-STALE updated_at (the row moved on the
	// write above) must be rejected as a conflict, not silently applied.
	_, err = repo.UpsertIfUnchanged(ctx, userID, []string{"backend"}, []string{"go", "docker", "kubernetes"}, []string{}, []string{}, []string{}, []string{}, []string{"en", "ru"}, nil, *created.UpdatedAt)
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

func TestSave_IncompleteProfileRoundTrip(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, "profile-partial@example.test").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	service := userprofile.New(userprofile.NewQueriesRepository(db.New(pool)))
	for _, values := range []struct{ roles, skills []string }{
		{[]string{}, []string{}},
		{[]string{"backend"}, []string{}},
		{[]string{}, []string{"go"}},
		{[]string{}, []string{}},
	} {
		_, err := service.Save(ctx, userID, values.roles, values.skills, nil, nil, nil, nil, &userprofile.LocationPreferences{WorkModes: []string{"remote"}})
		if err != nil {
			t.Fatalf("Save(%v, %v): %v", values.roles, values.skills, err)
		}
		stored, err := service.Get(ctx, userID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(stored.Specializations) != len(values.roles) || len(stored.Skills) != len(values.skills) {
			t.Fatalf("round trip: roles=%v, skills=%v", stored.Specializations, stored.Skills)
		}
		if len(stored.LocationPreferences) == 0 {
			t.Fatal("location was lost on partial save")
		}
	}
}
