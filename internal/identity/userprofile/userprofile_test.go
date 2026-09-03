package userprofile_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/identity/userprofile"
)

// upsertArgs captures the primitive params the repository's Upsert is handed, so the
// service tests can assert normalization without a db.* params struct.
type upsertArgs struct {
	UserID              int64
	Specializations     []string
	Skills              []string
	Seniorities         []string
	ExcludedSkills      []string
	LocationPreferences json.RawMessage
}

// guardedUpsertArgs is upsertArgs plus the expectedUpdatedAt UpsertIfUnchanged guards
// its write on.
type guardedUpsertArgs struct {
	upsertArgs
	ExpectedUpdatedAt time.Time
}

// fakeRepo records the params it is handed and returns canned profiles/errors, so the
// service tests run without a database (the searchprofile_test.go precedent).
type fakeRepo struct {
	upserted     upsertArgs
	upsertCalled bool
	upsertErr    error
	upsertRet    userprofile.Profile

	// guardedUpserted/guardedUpsertCalls record UpsertIfUnchanged's calls; guardedUpsertErrs
	// lets a test script a different result per call (e.g. ErrConflict then success), so
	// MergeSkills' retry loop can be exercised deterministically.
	guardedUpserted    guardedUpsertArgs
	guardedUpsertCalls int
	guardedUpsertErrs  []error
	guardedUpsertRet   userprofile.Profile

	getUserID int64
	getCalls  int
	getRet    userprofile.Profile
	getErr    error

	delUserID int64
	delCalled bool
	delErr    error
}

func (f *fakeRepo) Get(_ context.Context, userID int64) (userprofile.Profile, error) {
	f.getUserID = userID
	f.getCalls++
	return f.getRet, f.getErr
}

func (f *fakeRepo) Upsert(_ context.Context, userID int64, specializations, skills, seniorities, excludedSkills []string, locationPreferences json.RawMessage) (userprofile.Profile, error) {
	f.upserted = upsertArgs{UserID: userID, Specializations: specializations, Skills: skills, Seniorities: seniorities, ExcludedSkills: excludedSkills, LocationPreferences: locationPreferences}
	f.upsertCalled = true
	return f.upsertRet, f.upsertErr
}

func (f *fakeRepo) UpsertIfUnchanged(_ context.Context, userID int64, specializations, skills, seniorities, excludedSkills []string, locationPreferences json.RawMessage, expectedUpdatedAt time.Time) (userprofile.Profile, error) {
	f.guardedUpserted = guardedUpsertArgs{
		upsertArgs{UserID: userID, Specializations: specializations, Skills: skills, Seniorities: seniorities, ExcludedSkills: excludedSkills, LocationPreferences: locationPreferences},
		expectedUpdatedAt,
	}
	idx := f.guardedUpsertCalls
	f.guardedUpsertCalls++
	if idx < len(f.guardedUpsertErrs) && f.guardedUpsertErrs[idx] != nil {
		return userprofile.Profile{}, f.guardedUpsertErrs[idx]
	}
	return f.guardedUpsertRet, nil
}

func (f *fakeRepo) Delete(_ context.Context, userID int64) error {
	f.delUserID, f.delCalled = userID, true
	return f.delErr
}

func TestSave_UpsertsWithOwnerNormalizedSpecializationsAndSkills(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	svc := userprofile.New(repo)

	_, err := svc.Save(context.Background(), 7,
		[]string{" backend ", "devops", "backend"}, []string{"Go", " PostgreSQL ", "go"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !repo.upsertCalled {
		t.Fatal("repo.Upsert was not called")
	}
	if repo.upserted.UserID != 7 {
		t.Errorf("UserID = %d, want 7", repo.upserted.UserID)
	}
	wantSpec := []string{"backend", "devops"}
	if strings.Join(repo.upserted.Specializations, ",") != strings.Join(wantSpec, ",") {
		t.Errorf("Specializations = %v, want trimmed/deduped %v", repo.upserted.Specializations, wantSpec)
	}
	wantSkills := []string{"go", "postgresql"}
	if strings.Join(repo.upserted.Skills, ",") != strings.Join(wantSkills, ",") {
		t.Errorf("Skills = %v, want lowercased/trimmed/deduped %v", repo.upserted.Skills, wantSkills)
	}
}

func TestSave_RejectsUnknownSpecialization(t *testing.T) {
	repo := &fakeRepo{}
	_, err := userprofile.New(repo).Save(context.Background(), 7, []string{"backend", "wizardry"}, []string{"go"}, nil, nil, nil)
	if !errors.Is(err, userprofile.ErrInvalidSpecialization) {
		t.Errorf("err = %v, want ErrInvalidSpecialization", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called on an unknown specialization")
	}
}

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

func TestSave_RejectsEmptySpecializations(t *testing.T) {
	cases := []struct {
		name string
		in   []string
	}{
		{"nil", nil},
		{"empty slice", []string{}},
		{"only blanks", []string{"  ", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := userprofile.New(repo).Save(context.Background(), 7, tc.in, []string{"go"}, nil, nil, nil)
			if !errors.Is(err, userprofile.ErrEmptySpecializations) {
				t.Errorf("err = %v, want ErrEmptySpecializations", err)
			}
			if repo.upsertCalled {
				t.Error("repo.Upsert should not be called on empty specializations")
			}
		})
	}
}

func TestSave_RejectsTooManySpecializations(t *testing.T) {
	repo := &fakeRepo{}
	six := []string{"backend", "frontend", "fullstack", "mobile", "devops", "sre"}
	_, err := userprofile.New(repo).Save(context.Background(), 7, six, []string{"go"}, nil, nil, nil)
	if !errors.Is(err, userprofile.ErrTooManySpecializations) {
		t.Errorf("err = %v, want ErrTooManySpecializations", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called past the specialization cap")
	}
}

func TestSave_RejectsEmptySkills(t *testing.T) {
	cases := []struct {
		name string
		in   []string
	}{
		{"nil", nil},
		{"empty slice", []string{}},
		{"only blanks", []string{"  ", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := userprofile.New(repo).Save(context.Background(), 7, []string{"backend"}, tc.in, nil, nil, nil)
			if !errors.Is(err, userprofile.ErrEmptySkills) {
				t.Errorf("err = %v, want ErrEmptySkills", err)
			}
			if repo.upsertCalled {
				t.Error("repo.Upsert should not be called on empty skills")
			}
		})
	}
}

// manySkills builds n distinct skills, enough to cross a cardinality cap.
func manySkills(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "skill" + strconv.Itoa(i)
	}
	return out
}

// The saved skill list becomes one `skills != "…"` AND group per element in the coverage
// verdict's Meilisearch filter, so an unbounded list is a stored amplification of every
// later read against the index that also serves public search.
func TestSave_RejectsTooManySkills(t *testing.T) {
	repo := &fakeRepo{}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, manySkills(201), nil, nil, nil)
	if !errors.Is(err, userprofile.ErrTooManySkills) {
		t.Errorf("err = %v, want ErrTooManySkills", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called past the skill cap")
	}
}

func TestSave_AcceptsSkillsAtTheCap(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, manySkills(200), nil, nil, nil)
	if err != nil {
		t.Fatalf("Save at the cap: %v", err)
	}
	if len(repo.upserted.Skills) != 200 {
		t.Errorf("persisted %d skills, want all 200 at the cap", len(repo.upserted.Skills))
	}
}

func TestSave_RejectsTooManyExcludedSkills(t *testing.T) {
	repo := &fakeRepo{}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, []string{"go"}, nil, manySkills(201), nil)
	if !errors.Is(err, userprofile.ErrTooManySkills) {
		t.Errorf("err = %v, want ErrTooManySkills", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called past the excluded-skill cap")
	}
}

// A value far longer than any canonical skill is junk, not a skill, and is dropped the way
// blanks and duplicates already are — a per-value problem does not fail the whole save.
func TestSave_DropsOverlongSkills(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, []string{"go", strings.Repeat("x", 200)}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := []string{"go"}
	if strings.Join(repo.upserted.Skills, ",") != strings.Join(want, ",") {
		t.Errorf("Skills = %v, want the overlong value dropped %v", repo.upserted.Skills, want)
	}
}

func TestSave_NormalizesExcludedSkills(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, []string{"go"}, nil, []string{" PHP ", "php", "WordPress"}, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := []string{"php", "wordpress"}
	if strings.Join(repo.upserted.ExcludedSkills, ",") != strings.Join(want, ",") {
		t.Errorf("ExcludedSkills = %v, want lowercased/trimmed/deduped %v", repo.upserted.ExcludedSkills, want)
	}
}

func TestSave_DropsExcludedSkillAlsoInSkills(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, []string{"Go", "php"}, nil, []string{"go", "wordpress"}, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// "go" is wanted, so it is dropped from the excluded set; "wordpress" survives.
	want := []string{"wordpress"}
	if strings.Join(repo.upserted.ExcludedSkills, ",") != strings.Join(want, ",") {
		t.Errorf("ExcludedSkills = %v, want %v (wanted skill dropped)", repo.upserted.ExcludedSkills, want)
	}
}

func TestSave_EmptyExcludedSkillsStoresEmptySet(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, []string{"go"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if repo.upserted.ExcludedSkills == nil {
		t.Error("ExcludedSkills = nil, want non-nil empty set (persists as '{}', not NULL)")
	}
	if len(repo.upserted.ExcludedSkills) != 0 {
		t.Errorf("ExcludedSkills = %v, want empty", repo.upserted.ExcludedSkills)
	}
}

func TestSave_RejectsUnknownSeniority(t *testing.T) {
	repo := &fakeRepo{}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, []string{"go"}, []string{"senior", "wizard"}, nil, nil)
	if !errors.Is(err, userprofile.ErrInvalidSeniority) {
		t.Errorf("err = %v, want ErrInvalidSeniority", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called on an unknown seniority")
	}
}

func TestSave_SenioritiesAreDeduplicated(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, []string{"go"}, []string{"senior", "middle", "senior"}, nil, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := []string{"senior", "middle"}
	if strings.Join(repo.upserted.Seniorities, ",") != strings.Join(want, ",") {
		t.Errorf("Seniorities = %v, want deduplicated, first-seen order %v", repo.upserted.Seniorities, want)
	}
}

func TestSave_SenioritiesMayBeEmpty(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, []string{"go"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if repo.upserted.Seniorities == nil {
		t.Error("Seniorities = nil, want non-nil empty set (persists as '{}', not NULL)")
	}
	if len(repo.upserted.Seniorities) != 0 {
		t.Errorf("Seniorities = %v, want empty", repo.upserted.Seniorities)
	}
}

func TestSave_AcceptsMultipleSeniorities(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	_, err := userprofile.New(repo).Save(context.Background(), 7,
		[]string{"backend"}, []string{"go"}, []string{"middle", "senior"}, nil, nil)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := []string{"middle", "senior"}
	if strings.Join(repo.upserted.Seniorities, ",") != strings.Join(want, ",") {
		t.Errorf("Seniorities = %v, want %v", repo.upserted.Seniorities, want)
	}
}

func TestGet_ReturnsOwnersProfile(t *testing.T) {
	repo := &fakeRepo{getRet: userprofile.Profile{UserID: 7}}
	got, err := userprofile.New(repo).Get(context.Background(), 7)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UserID != 7 {
		t.Errorf("got profile user %d, want 7", got.UserID)
	}
	if repo.getUserID != 7 {
		t.Errorf("get scope = user %d, want 7", repo.getUserID)
	}
}

func TestGet_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: userprofile.ErrNotFound}
	_, err := userprofile.New(repo).Get(context.Background(), 7)
	if !errors.Is(err, userprofile.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDelete_IsIdempotentAndScopedToOwner(t *testing.T) {
	repo := &fakeRepo{}
	if err := userprofile.New(repo).Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !repo.delCalled || repo.delUserID != 7 {
		t.Errorf("delete scope = user %d (called=%v), want user 7", repo.delUserID, repo.delCalled)
	}
}
