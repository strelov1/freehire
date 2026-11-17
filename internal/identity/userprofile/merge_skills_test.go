package userprofile_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/identity/userprofile"
)

func TestMergeSkills_NoopWhenNoProfile(t *testing.T) {
	repo := &fakeRepo{getErr: userprofile.ErrNotFound}
	err := userprofile.New(repo).MergeSkills(context.Background(), 7, []string{"docker"})
	if err != nil {
		t.Fatalf("MergeSkills: %v", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called when the user has no profile")
	}
}

func TestMergeSkills_AddsNewSkills(t *testing.T) {
	repo := &fakeRepo{getRet: userprofile.Profile{
		UserID:          7,
		Specializations: []string{"backend"},
		Skills:          []string{"go"},
		ExcludedSkills:  []string{"php"},
	}}
	err := userprofile.New(repo).MergeSkills(context.Background(), 7, []string{"Docker", "kubernetes"})
	if err != nil {
		t.Fatalf("MergeSkills: %v", err)
	}
	if !repo.upsertCalled {
		t.Fatal("repo.Upsert was not called")
	}
	wantSkills := []string{"go", "docker", "kubernetes"}
	if strings.Join(repo.upserted.Skills, ",") != strings.Join(wantSkills, ",") {
		t.Errorf("Skills = %v, want %v", repo.upserted.Skills, wantSkills)
	}
	// Everything else the profile held must round-trip unchanged.
	if strings.Join(repo.upserted.Specializations, ",") != "backend" {
		t.Errorf("Specializations = %v, want unchanged [backend]", repo.upserted.Specializations)
	}
	if strings.Join(repo.upserted.ExcludedSkills, ",") != "php" {
		t.Errorf("ExcludedSkills = %v, want unchanged [php]", repo.upserted.ExcludedSkills)
	}
}

func TestMergeSkills_SkipsCaseInsensitiveDuplicate(t *testing.T) {
	repo := &fakeRepo{getRet: userprofile.Profile{UserID: 7, Skills: []string{"go", "docker"}}}
	err := userprofile.New(repo).MergeSkills(context.Background(), 7, []string{"GO", "Docker"})
	if err != nil {
		t.Fatalf("MergeSkills: %v", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called when every skill is already present")
	}
}

func TestMergeSkills_SkipsExcludedSkill(t *testing.T) {
	repo := &fakeRepo{getRet: userprofile.Profile{
		UserID:         7,
		Skills:         []string{"go"},
		ExcludedSkills: []string{"php"},
	}}
	err := userprofile.New(repo).MergeSkills(context.Background(), 7, []string{"PHP"})
	if err != nil {
		t.Fatalf("MergeSkills: %v", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called when the only new skill is excluded")
	}
}

func TestMergeSkills_NeverExceedsCap(t *testing.T) {
	// Mirrors the 200 cap documented on maxSkills (internal/identity/userprofile.go).
	existing := make([]string, 199)
	for i := range existing {
		existing[i] = "skill" + strconv.Itoa(i)
	}
	repo := &fakeRepo{getRet: userprofile.Profile{UserID: 7, Skills: existing}}
	err := userprofile.New(repo).MergeSkills(context.Background(), 7, []string{"new-a", "new-b", "new-c"})
	if err != nil {
		t.Fatalf("MergeSkills: %v", err)
	}
	if !repo.upsertCalled {
		t.Fatal("repo.Upsert was not called")
	}
	if len(repo.upserted.Skills) != 200 {
		t.Fatalf("len(Skills) = %d, want 200 (cap)", len(repo.upserted.Skills))
	}
	if repo.upserted.Skills[199] != "new-a" {
		t.Errorf("Skills[199] = %q, want the first skill that fit under the cap", repo.upserted.Skills[199])
	}
}

// A profile read WITH a known updated_at must write through the guarded
// UpsertIfUnchanged, not the unconditional Upsert — this is the fix itself: a merge
// computed outside a transaction must not blindly clobber a concurrent Save().
func TestMergeSkills_UsesGuardedWriteWhenUpdatedAtKnown(t *testing.T) {
	updatedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{getRet: userprofile.Profile{
		UserID: 7, Skills: []string{"go"}, Specializations: []string{"backend"}, UpdatedAt: &updatedAt,
	}}
	err := userprofile.New(repo).MergeSkills(context.Background(), 7, []string{"docker"})
	if err != nil {
		t.Fatalf("MergeSkills: %v", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert (unguarded) should not be called when updated_at is known")
	}
	if repo.guardedUpsertCalls != 1 {
		t.Fatalf("guarded upsert calls = %d, want 1", repo.guardedUpsertCalls)
	}
	if !repo.guardedUpserted.ExpectedUpdatedAt.Equal(updatedAt) {
		t.Errorf("guarded write's expectedUpdatedAt = %v, want %v", repo.guardedUpserted.ExpectedUpdatedAt, updatedAt)
	}
	wantSkills := []string{"go", "docker"}
	if strings.Join(repo.guardedUpserted.Skills, ",") != strings.Join(wantSkills, ",") {
		t.Errorf("Skills = %v, want %v", repo.guardedUpserted.Skills, wantSkills)
	}
}

// The race the review flagged: a concurrent Save() commits between MergeSkills' Get and
// its write, so the guarded write reports ErrConflict once. MergeSkills re-reads and
// retries rather than giving up or (worse) silently reverting the concurrent Save().
func TestMergeSkills_RetriesOnConflictThenSucceeds(t *testing.T) {
	updatedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		getRet:            userprofile.Profile{UserID: 7, Skills: []string{"go"}, UpdatedAt: &updatedAt},
		guardedUpsertErrs: []error{userprofile.ErrConflict, nil},
	}
	err := userprofile.New(repo).MergeSkills(context.Background(), 7, []string{"docker"})
	if err != nil {
		t.Fatalf("MergeSkills: %v", err)
	}
	if repo.guardedUpsertCalls != 2 {
		t.Errorf("guarded upsert calls = %d, want 2 (one conflict, one success)", repo.guardedUpsertCalls)
	}
	if repo.getCalls != 2 {
		t.Errorf("Get calls = %d, want 2 — a retry must re-read the profile, not reuse the stale snapshot", repo.getCalls)
	}
}

// A concurrent writer that keeps winning the race exhausts MergeSkills' bounded retry:
// it gives up and reports ErrConflict rather than retrying forever or, worse, forcing a
// write that would revert whatever kept winning.
func TestMergeSkills_GivesUpAfterRepeatedConflicts(t *testing.T) {
	updatedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		getRet:            userprofile.Profile{UserID: 7, Skills: []string{"go"}, UpdatedAt: &updatedAt},
		guardedUpsertErrs: []error{userprofile.ErrConflict, userprofile.ErrConflict, userprofile.ErrConflict},
	}
	err := userprofile.New(repo).MergeSkills(context.Background(), 7, []string{"docker"})
	if !errors.Is(err, userprofile.ErrConflict) {
		t.Errorf("err = %v, want ErrConflict after exhausting retries", err)
	}
	if repo.guardedUpsertCalls != 3 {
		t.Errorf("guarded upsert calls = %d, want 3 (mergeSkillsMaxAttempts)", repo.guardedUpsertCalls)
	}
}
