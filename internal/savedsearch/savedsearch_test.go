package savedsearch_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/savedsearch"
)

// fakeRepo records the params it is handed and returns canned rows/errors, so the
// service tests run without a database (the submission_test.go precedent).
type fakeRepo struct {
	count    int64
	countErr error

	created      db.CreateSavedSearchParams
	createCalled bool
	createErr    error
	createRet    db.SavedSearch

	updated      db.UpdateSavedSearchParams
	updateCalled bool
	updateErr    error
	updateRet    db.SavedSearch

	deleted      db.DeleteSavedSearchParams
	deleteCalled bool
	deleteErr    error

	listRet []db.SavedSearch
}

func (f *fakeRepo) List(_ context.Context, _ int64) ([]db.SavedSearch, error) {
	return f.listRet, nil
}

func (f *fakeRepo) Count(_ context.Context, _ int64) (int64, error) {
	return f.count, f.countErr
}

func (f *fakeRepo) Create(_ context.Context, p db.CreateSavedSearchParams) (db.SavedSearch, error) {
	f.created, f.createCalled = p, true
	return f.createRet, f.createErr
}

func (f *fakeRepo) Update(_ context.Context, p db.UpdateSavedSearchParams) (db.SavedSearch, error) {
	f.updated, f.updateCalled = p, true
	return f.updateRet, f.updateErr
}

func (f *fakeRepo) Delete(_ context.Context, p db.DeleteSavedSearchParams) error {
	f.deleted, f.deleteCalled = p, true
	return f.deleteErr
}

func TestCreate_PersistsWithOwnerAndTrimmedName(t *testing.T) {
	repo := &fakeRepo{createRet: db.SavedSearch{ID: 1}}
	svc := savedsearch.New(repo)

	_, err := svc.Create(context.Background(), 7, "  Remote Go  ", "q=go&work_mode=remote")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("repo.Create was not called")
	}
	if repo.created.UserID != 7 {
		t.Errorf("UserID = %d, want 7", repo.created.UserID)
	}
	if repo.created.Name != "Remote Go" {
		t.Errorf("Name = %q, want trimmed %q", repo.created.Name, "Remote Go")
	}
	if repo.created.Query != "q=go&work_mode=remote" {
		t.Errorf("Query = %q, not carried through", repo.created.Query)
	}
}

func TestCreate_EmptyQueryAllowed(t *testing.T) {
	repo := &fakeRepo{createRet: db.SavedSearch{ID: 1}}
	_, err := savedsearch.New(repo).Create(context.Background(), 7, "All jobs", "")
	if err != nil {
		t.Fatalf("Create with empty query: %v", err)
	}
	if repo.created.Query != "" {
		t.Errorf("Query = %q, want empty (show-all)", repo.created.Query)
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
			_, err := savedsearch.New(repo).Create(context.Background(), 7, tc.in, "")
			if !errors.Is(err, savedsearch.ErrInvalidName) {
				t.Errorf("err = %v, want ErrInvalidName", err)
			}
			if repo.createCalled {
				t.Error("repo.Create should not be called on an invalid name")
			}
		})
	}
}

func TestCreate_NameLengthCountsRunes(t *testing.T) {
	// The DB CHECK bounds length(trim(name)) in characters, and the app is RU-facing,
	// so the name limit must count runes, not bytes — a 100-rune Cyrillic name (200
	// bytes) is valid; 101 runes is not.
	repo := &fakeRepo{createRet: db.SavedSearch{ID: 1}}
	if _, err := savedsearch.New(repo).Create(context.Background(), 7, strings.Repeat("я", 100), ""); err != nil {
		t.Errorf("100-rune name: err = %v, want nil", err)
	}
	if !repo.createCalled {
		t.Error("repo.Create should be called for a valid 100-rune name")
	}

	repo = &fakeRepo{}
	if _, err := savedsearch.New(repo).Create(context.Background(), 7, strings.Repeat("я", 101), ""); !errors.Is(err, savedsearch.ErrInvalidName) {
		t.Errorf("101-rune name: err = %v, want ErrInvalidName", err)
	}
}

func TestCreate_EnforcesCap(t *testing.T) {
	repo := &fakeRepo{count: 50} // already at the cap
	_, err := savedsearch.New(repo).Create(context.Background(), 7, "One more", "")
	if !errors.Is(err, savedsearch.ErrCapExceeded) {
		t.Errorf("err = %v, want ErrCapExceeded", err)
	}
	if repo.createCalled {
		t.Error("repo.Create should not be called once the cap is reached")
	}
}

func TestCreate_PropagatesDuplicateName(t *testing.T) {
	repo := &fakeRepo{createErr: savedsearch.ErrDuplicateName}
	_, err := savedsearch.New(repo).Create(context.Background(), 7, "Dup", "")
	if !errors.Is(err, savedsearch.ErrDuplicateName) {
		t.Errorf("err = %v, want ErrDuplicateName", err)
	}
}

func TestUpdate_PartialFields(t *testing.T) {
	repo := &fakeRepo{updateRet: db.SavedSearch{ID: 5}}
	svc := savedsearch.New(repo)

	newName := "  Renamed  "
	_, err := svc.Update(context.Background(), 7, 5, &newName, nil)
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
	if repo.updated.Query.Valid {
		t.Error("Query param should be NULL (unchanged) when not provided")
	}
}

func TestUpdate_RejectsInvalidName(t *testing.T) {
	repo := &fakeRepo{}
	blank := "  "
	_, err := savedsearch.New(repo).Update(context.Background(), 7, 5, &blank, nil)
	if !errors.Is(err, savedsearch.ErrInvalidName) {
		t.Errorf("err = %v, want ErrInvalidName", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called on an invalid name")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := &fakeRepo{updateErr: savedsearch.ErrNotFound}
	q := "q=go"
	_, err := savedsearch.New(repo).Update(context.Background(), 7, 999, nil, &q)
	if !errors.Is(err, savedsearch.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDelete_ScopedToOwner(t *testing.T) {
	repo := &fakeRepo{}
	err := savedsearch.New(repo).Delete(context.Background(), 7, 5)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if repo.deleted.ID != 5 || repo.deleted.UserID != 7 {
		t.Errorf("delete scope = id %d user %d, want id 5 user 7", repo.deleted.ID, repo.deleted.UserID)
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := &fakeRepo{deleteErr: savedsearch.ErrNotFound}
	err := savedsearch.New(repo).Delete(context.Background(), 7, 999)
	if !errors.Is(err, savedsearch.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
