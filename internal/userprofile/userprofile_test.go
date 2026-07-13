package userprofile_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/userprofile"
)

// upsertArgs captures the primitive params the repository's Upsert is handed, so the
// service tests can assert normalization without a db.* params struct.
type upsertArgs struct {
	UserID              int64
	Specializations     []string
	Skills              []string
	LocationPreferences json.RawMessage
}

// fakeRepo records the params it is handed and returns canned profiles/errors, so the
// service tests run without a database (the searchprofile_test.go precedent).
type fakeRepo struct {
	upserted     upsertArgs
	upsertCalled bool
	upsertErr    error
	upsertRet    userprofile.Profile

	getUserID int64
	getRet    userprofile.Profile
	getErr    error

	delUserID int64
	delCalled bool
	delErr    error
}

func (f *fakeRepo) Get(_ context.Context, userID int64) (userprofile.Profile, error) {
	f.getUserID = userID
	return f.getRet, f.getErr
}

func (f *fakeRepo) Upsert(_ context.Context, userID int64, specializations, skills []string, locationPreferences json.RawMessage) (userprofile.Profile, error) {
	f.upserted = upsertArgs{UserID: userID, Specializations: specializations, Skills: skills, LocationPreferences: locationPreferences}
	f.upsertCalled = true
	return f.upsertRet, f.upsertErr
}

func (f *fakeRepo) Delete(_ context.Context, userID int64) error {
	f.delUserID, f.delCalled = userID, true
	return f.delErr
}

func TestSave_UpsertsWithOwnerNormalizedSpecializationsAndSkills(t *testing.T) {
	repo := &fakeRepo{upsertRet: userprofile.Profile{UserID: 7}}
	svc := userprofile.New(repo)

	_, err := svc.Save(context.Background(), 7,
		[]string{" backend ", "devops", "backend"}, []string{"Go", " PostgreSQL ", "go"}, nil)
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
	_, err := userprofile.New(repo).Save(context.Background(), 7, []string{"backend", "wizardry"}, []string{"go"}, nil)
	if !errors.Is(err, userprofile.ErrInvalidSpecialization) {
		t.Errorf("err = %v, want ErrInvalidSpecialization", err)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called on an unknown specialization")
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
			_, err := userprofile.New(repo).Save(context.Background(), 7, tc.in, []string{"go"}, nil)
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
	_, err := userprofile.New(repo).Save(context.Background(), 7, six, []string{"go"}, nil)
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
			_, err := userprofile.New(repo).Save(context.Background(), 7, []string{"backend"}, tc.in, nil)
			if !errors.Is(err, userprofile.ErrEmptySkills) {
				t.Errorf("err = %v, want ErrEmptySkills", err)
			}
			if repo.upsertCalled {
				t.Error("repo.Upsert should not be called on empty skills")
			}
		})
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
