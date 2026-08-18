//go:build integration

// Integration test for ListUserProfilesExcludedSkills — the batch fetch behind
// internal/notify's per-subscriber avoid-skills enforcement. Only a real Postgres can
// verify the `= ANY($1::bigint[])` array-parameter behavior, including that a user id
// with no profile row simply produces no result row. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

func insertUserProfileForExcludedSkillsTest(t *testing.T, q *Queries, email string, excludedSkills []string) int64 {
	t.Helper()
	var userID int64
	if err := q.db.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	if _, err := q.UpsertUserProfile(context.Background(), UpsertUserProfileParams{
		UserID:          userID,
		Specializations: []string{"backend"},
		Skills:          []string{"go"},
		ExcludedSkills:  excludedSkills,
	}); err != nil {
		t.Fatalf("seed profile for %s: %v", email, err)
	}
	return userID
}

func TestListUserProfilesExcludedSkills(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := insertUserProfileForExcludedSkillsTest(t, q, "alice-excluded-skills@example.test", []string{"php", "java"})
	bob := insertUserProfileForExcludedSkillsTest(t, q, "bob-excluded-skills@example.test", []string{})

	// carol has a users row but no profile at all.
	var carol int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, "carol-excluded-skills@example.test").Scan(&carol); err != nil {
		t.Fatalf("insert carol: %v", err)
	}

	rows, err := q.ListUserProfilesExcludedSkills(ctx, []int64{alice, bob, carol})
	if err != nil {
		t.Fatalf("ListUserProfilesExcludedSkills: %v", err)
	}

	byUser := make(map[int64][]string, len(rows))
	for _, r := range rows {
		byUser[r.UserID] = r.ExcludedSkills
	}

	if got := byUser[alice]; len(got) != 2 || got[0] != "php" || got[1] != "java" {
		t.Errorf("alice's excluded skills = %v, want [php java]", got)
	}
	if got, ok := byUser[bob]; !ok || len(got) != 0 {
		t.Errorf("bob's excluded skills = %v (present=%v), want present and empty", got, ok)
	}
	if _, ok := byUser[carol]; ok {
		t.Errorf("carol has no profile row, want absent from the result, got a row")
	}
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2 (alice + bob; carol has no profile row)", len(rows))
	}
}
