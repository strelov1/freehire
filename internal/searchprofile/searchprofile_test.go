package searchprofile_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/searchprofile"
)

// fakeRepo records the params it is handed and returns canned rows/errors, so the
// service tests run without a database (the savedsearch_test.go precedent).
type fakeRepo struct {
	count    int64
	countErr error

	created      db.CreateSearchProfileParams
	createCalled bool
	createErr    error
	createRet    db.SearchProfile

	updated      db.UpdateSearchProfileParams
	updateCalled bool
	updateErr    error
	updateRet    db.SearchProfile

	deleted      db.DeleteSearchProfileParams
	deleteCalled bool
	deleteErr    error

	getParams db.GetSearchProfileParams
	getRet    db.SearchProfile
	getErr    error

	listRet []db.SearchProfile
}

func (f *fakeRepo) Get(_ context.Context, p db.GetSearchProfileParams) (db.SearchProfile, error) {
	f.getParams = p
	return f.getRet, f.getErr
}

func (f *fakeRepo) List(_ context.Context, _ int64) ([]db.SearchProfile, error) {
	return f.listRet, nil
}

func (f *fakeRepo) Count(_ context.Context, _ int64) (int64, error) {
	return f.count, f.countErr
}

func (f *fakeRepo) Create(_ context.Context, p db.CreateSearchProfileParams) (db.SearchProfile, error) {
	f.created, f.createCalled = p, true
	return f.createRet, f.createErr
}

func (f *fakeRepo) Update(_ context.Context, p db.UpdateSearchProfileParams) (db.SearchProfile, error) {
	f.updated, f.updateCalled = p, true
	return f.updateRet, f.updateErr
}

func (f *fakeRepo) Delete(_ context.Context, p db.DeleteSearchProfileParams) error {
	f.deleted, f.deleteCalled = p, true
	return f.deleteErr
}

func ptr(s string) *string { return &s }

func TestCreate_PersistsWithOwnerTrimmedNameNormalizedSpecializationsAndSkills(t *testing.T) {
	repo := &fakeRepo{createRet: db.SearchProfile{ID: 1}}
	svc := searchprofile.New(repo)

	_, err := svc.Create(context.Background(), 7, "  Go backend  ",
		[]string{" backend ", "devops", "backend"}, []string{"Go", " PostgreSQL ", "go"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("repo.Create was not called")
	}
	if repo.created.UserID != 7 {
		t.Errorf("UserID = %d, want 7", repo.created.UserID)
	}
	if repo.created.Name != "Go backend" {
		t.Errorf("Name = %q, want trimmed %q", repo.created.Name, "Go backend")
	}
	wantSpec := []string{"backend", "devops"}
	if strings.Join(repo.created.Specializations, ",") != strings.Join(wantSpec, ",") {
		t.Errorf("Specializations = %v, want trimmed/deduped %v", repo.created.Specializations, wantSpec)
	}
	wantSkills := []string{"go", "postgresql"}
	if strings.Join(repo.created.Skills, ",") != strings.Join(wantSkills, ",") {
		t.Errorf("Skills = %v, want lowercased/trimmed/deduped %v", repo.created.Skills, wantSkills)
	}
}

func TestCreate_RejectsInvalidName(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"too long", strings.Repeat("x", 101)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := searchprofile.New(repo).Create(context.Background(), 7, tc.in, []string{"backend"}, []string{"go"})
			if !errors.Is(err, searchprofile.ErrInvalidName) {
				t.Errorf("err = %v, want ErrInvalidName", err)
			}
			if repo.createCalled {
				t.Error("repo.Create should not be called on an invalid name")
			}
		})
	}
}

func TestCreate_NameLengthCountsRunes(t *testing.T) {
	repo := &fakeRepo{createRet: db.SearchProfile{ID: 1}}
	if _, err := searchprofile.New(repo).Create(context.Background(), 7, strings.Repeat("я", 100), []string{"backend"}, []string{"go"}); err != nil {
		t.Errorf("100-rune name: err = %v, want nil", err)
	}
	if !repo.createCalled {
		t.Error("repo.Create should be called for a valid 100-rune name")
	}

	repo = &fakeRepo{}
	if _, err := searchprofile.New(repo).Create(context.Background(), 7, strings.Repeat("я", 101), []string{"backend"}, []string{"go"}); !errors.Is(err, searchprofile.ErrInvalidName) {
		t.Errorf("101-rune name: err = %v, want ErrInvalidName", err)
	}
}

func TestCreate_RejectsUnknownSpecialization(t *testing.T) {
	repo := &fakeRepo{}
	_, err := searchprofile.New(repo).Create(context.Background(), 7, "Bad", []string{"backend", "wizardry"}, []string{"go"})
	if !errors.Is(err, searchprofile.ErrInvalidSpecialization) {
		t.Errorf("err = %v, want ErrInvalidSpecialization", err)
	}
	if repo.createCalled {
		t.Error("repo.Create should not be called on an unknown specialization")
	}
}

func TestCreate_RejectsEmptySpecializations(t *testing.T) {
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
			_, err := searchprofile.New(repo).Create(context.Background(), 7, "Empty", tc.in, []string{"go"})
			if !errors.Is(err, searchprofile.ErrEmptySpecializations) {
				t.Errorf("err = %v, want ErrEmptySpecializations", err)
			}
			if repo.createCalled {
				t.Error("repo.Create should not be called on empty specializations")
			}
		})
	}
}

func TestCreate_RejectsTooManySpecializations(t *testing.T) {
	repo := &fakeRepo{}
	six := []string{"backend", "frontend", "fullstack", "mobile", "devops", "sre"}
	_, err := searchprofile.New(repo).Create(context.Background(), 7, "Too many", six, []string{"go"})
	if !errors.Is(err, searchprofile.ErrTooManySpecializations) {
		t.Errorf("err = %v, want ErrTooManySpecializations", err)
	}
	if repo.createCalled {
		t.Error("repo.Create should not be called past the specialization cap")
	}
}

func TestCreate_RejectsEmptySkills(t *testing.T) {
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
			_, err := searchprofile.New(repo).Create(context.Background(), 7, "Empty", []string{"backend"}, tc.in)
			if !errors.Is(err, searchprofile.ErrEmptySkills) {
				t.Errorf("err = %v, want ErrEmptySkills", err)
			}
			if repo.createCalled {
				t.Error("repo.Create should not be called on empty skills")
			}
		})
	}
}

func TestCreate_EnforcesCap(t *testing.T) {
	repo := &fakeRepo{count: 50} // already at the cap
	_, err := searchprofile.New(repo).Create(context.Background(), 7, "One more", []string{"backend"}, []string{"go"})
	if !errors.Is(err, searchprofile.ErrCapExceeded) {
		t.Errorf("err = %v, want ErrCapExceeded", err)
	}
	if repo.createCalled {
		t.Error("repo.Create should not be called once the cap is reached")
	}
}

func TestCreate_PropagatesDuplicateName(t *testing.T) {
	repo := &fakeRepo{createErr: searchprofile.ErrDuplicateName}
	_, err := searchprofile.New(repo).Create(context.Background(), 7, "Dup", []string{"backend"}, []string{"go"})
	if !errors.Is(err, searchprofile.ErrDuplicateName) {
		t.Errorf("err = %v, want ErrDuplicateName", err)
	}
}

func TestUpdate_PartialRename_LeavesOtherFieldsUnchanged(t *testing.T) {
	repo := &fakeRepo{updateRet: db.SearchProfile{ID: 5}}
	svc := searchprofile.New(repo)

	_, err := svc.Update(context.Background(), 7, 5, ptr("  Renamed  "), nil, nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("repo.Update was not called")
	}
	if repo.updated.ID != 5 || repo.updated.UserID != 7 {
		t.Errorf("update scope = id %d user %d, want id 5 user 7", repo.updated.ID, repo.updated.UserID)
	}
	if !repo.updated.Name.Valid || repo.updated.Name.String != "Renamed" {
		t.Errorf("Name param = %+v, want trimmed valid %q", repo.updated.Name, "Renamed")
	}
	if repo.updated.Specializations != nil {
		t.Error("Specializations param should be nil (unchanged) when not provided")
	}
	if repo.updated.Skills != nil {
		t.Error("Skills param should be nil (unchanged) when not provided")
	}
}

func TestUpdate_ReplaceSpecializations_Normalized(t *testing.T) {
	repo := &fakeRepo{updateRet: db.SearchProfile{ID: 5}}
	_, err := searchprofile.New(repo).Update(context.Background(), 7, 5, nil, []string{" devops ", "sre", "devops"}, nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	want := []string{"devops", "sre"}
	if strings.Join(repo.updated.Specializations, ",") != strings.Join(want, ",") {
		t.Errorf("Specializations param = %v, want %v", repo.updated.Specializations, want)
	}
}

func TestUpdate_ReplaceSkills_Normalized(t *testing.T) {
	repo := &fakeRepo{updateRet: db.SearchProfile{ID: 5}}
	_, err := searchprofile.New(repo).Update(context.Background(), 7, 5, nil, nil, []string{"Docker", "docker", " K8s "})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	want := []string{"docker", "k8s"}
	if strings.Join(repo.updated.Skills, ",") != strings.Join(want, ",") {
		t.Errorf("Skills param = %v, want %v", repo.updated.Skills, want)
	}
}

func TestUpdate_RejectsInvalidName(t *testing.T) {
	repo := &fakeRepo{}
	_, err := searchprofile.New(repo).Update(context.Background(), 7, 5, ptr("  "), nil, nil)
	if !errors.Is(err, searchprofile.ErrInvalidName) {
		t.Errorf("err = %v, want ErrInvalidName", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called on an invalid name")
	}
}

func TestUpdate_RejectsUnknownSpecialization(t *testing.T) {
	repo := &fakeRepo{}
	_, err := searchprofile.New(repo).Update(context.Background(), 7, 5, nil, []string{"wizardry"}, nil)
	if !errors.Is(err, searchprofile.ErrInvalidSpecialization) {
		t.Errorf("err = %v, want ErrInvalidSpecialization", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called on an unknown specialization")
	}
}

func TestUpdate_RejectsEmptySpecializationsWhenProvided(t *testing.T) {
	repo := &fakeRepo{}
	_, err := searchprofile.New(repo).Update(context.Background(), 7, 5, nil, []string{"  ", ""}, nil)
	if !errors.Is(err, searchprofile.ErrEmptySpecializations) {
		t.Errorf("err = %v, want ErrEmptySpecializations", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called when provided specializations reduce to empty")
	}
}

func TestUpdate_RejectsTooManySpecializations(t *testing.T) {
	repo := &fakeRepo{}
	six := []string{"backend", "frontend", "fullstack", "mobile", "devops", "sre"}
	_, err := searchprofile.New(repo).Update(context.Background(), 7, 5, nil, six, nil)
	if !errors.Is(err, searchprofile.ErrTooManySpecializations) {
		t.Errorf("err = %v, want ErrTooManySpecializations", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called past the specialization cap")
	}
}

func TestUpdate_RejectsEmptySkillsWhenProvided(t *testing.T) {
	repo := &fakeRepo{}
	_, err := searchprofile.New(repo).Update(context.Background(), 7, 5, nil, nil, []string{"  ", ""})
	if !errors.Is(err, searchprofile.ErrEmptySkills) {
		t.Errorf("err = %v, want ErrEmptySkills", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called when provided skills reduce to empty")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := &fakeRepo{updateErr: searchprofile.ErrNotFound}
	_, err := searchprofile.New(repo).Update(context.Background(), 7, 999, ptr("X"), nil, nil)
	if !errors.Is(err, searchprofile.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDelete_ScopedToOwner(t *testing.T) {
	repo := &fakeRepo{}
	err := searchprofile.New(repo).Delete(context.Background(), 7, 5)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if repo.deleted.ID != 5 || repo.deleted.UserID != 7 {
		t.Errorf("delete scope = id %d user %d, want id 5 user 7", repo.deleted.ID, repo.deleted.UserID)
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := &fakeRepo{deleteErr: searchprofile.ErrNotFound}
	err := searchprofile.New(repo).Delete(context.Background(), 7, 999)
	if !errors.Is(err, searchprofile.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGet_ScopedToOwner(t *testing.T) {
	repo := &fakeRepo{getRet: db.SearchProfile{ID: 5}}
	got, err := searchprofile.New(repo).Get(context.Background(), 7, 5)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != 5 {
		t.Errorf("got profile id %d, want 5", got.ID)
	}
	if repo.getParams.ID != 5 || repo.getParams.UserID != 7 {
		t.Errorf("get scope = id %d user %d, want id 5 user 7", repo.getParams.ID, repo.getParams.UserID)
	}
}

func TestGet_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: searchprofile.ErrNotFound}
	_, err := searchprofile.New(repo).Get(context.Background(), 7, 999)
	if !errors.Is(err, searchprofile.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
