package savedsearch_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/search/savedsearch"
)

// The *Args structs capture the primitive params the repository is handed, so the service
// tests can assert them without a db.* params struct.
type createArgs struct {
	UserID             int64
	Name               string
	Query              string
	DerivedFromProfile bool
}

type updateArgs struct {
	ID     int64
	UserID int64
	Name   *string
	Query  *string
}

type deleteArgs struct {
	ID     int64
	UserID int64
}

// fakeRepo records the params it is handed and returns canned rows/errors, so the
// service tests run without a database (the submission_test.go precedent).
type fakeRepo struct {
	count    int64
	countErr error

	created      createArgs
	createCalled bool
	createErr    error
	createRet    savedsearch.SavedSearch

	updated      updateArgs
	updateCalled bool
	updateErr    error
	updateRet    savedsearch.SavedSearch

	deleted      deleteArgs
	deleteCalled bool
	deleteErr    error

	listRet []savedsearch.SavedSearch
}

func (f *fakeRepo) List(_ context.Context, _ int64) ([]savedsearch.SavedSearch, error) {
	return f.listRet, nil
}

func (f *fakeRepo) Count(_ context.Context, _ int64) (int64, error) {
	return f.count, f.countErr
}

func (f *fakeRepo) Create(_ context.Context, userID int64, name, query string, derivedFromProfile bool) (savedsearch.SavedSearch, error) {
	f.created, f.createCalled = createArgs{UserID: userID, Name: name, Query: query, DerivedFromProfile: derivedFromProfile}, true
	return f.createRet, f.createErr
}

func (f *fakeRepo) Update(_ context.Context, id, userID int64, name, query *string) (savedsearch.SavedSearch, error) {
	f.updated, f.updateCalled = updateArgs{ID: id, UserID: userID, Name: name, Query: query}, true
	return f.updateRet, f.updateErr
}

func (f *fakeRepo) Delete(_ context.Context, id, userID int64) error {
	f.deleted, f.deleteCalled = deleteArgs{ID: id, UserID: userID}, true
	return f.deleteErr
}

func TestCreate_PersistsWithOwnerAndTrimmedName(t *testing.T) {
	repo := &fakeRepo{createRet: savedsearch.SavedSearch{ID: 1}}
	svc := savedsearch.New(repo)

	_, err := svc.Create(context.Background(), 7, "  Remote Go  ", "q=go&work_mode=remote", false)
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
	repo := &fakeRepo{createRet: savedsearch.SavedSearch{ID: 1}}
	_, err := savedsearch.New(repo).Create(context.Background(), 7, "All jobs", "", false)
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
			_, err := savedsearch.New(repo).Create(context.Background(), 7, tc.in, "", false)
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
	repo := &fakeRepo{createRet: savedsearch.SavedSearch{ID: 1}}
	if _, err := savedsearch.New(repo).Create(context.Background(), 7, strings.Repeat("я", 100), "", false); err != nil {
		t.Errorf("100-rune name: err = %v, want nil", err)
	}
	if !repo.createCalled {
		t.Error("repo.Create should be called for a valid 100-rune name")
	}

	repo = &fakeRepo{}
	if _, err := savedsearch.New(repo).Create(context.Background(), 7, strings.Repeat("я", 101), "", false); !errors.Is(err, savedsearch.ErrInvalidName) {
		t.Errorf("101-rune name: err = %v, want ErrInvalidName", err)
	}
}

// The query is a client-supplied URL-encoded filter string, re-parsed on every
// internal/engage/notify pass — the review finding was that, unlike name, it carried no
// length bound anywhere.
func TestCreate_RejectsOverLongQuery(t *testing.T) {
	repo := &fakeRepo{}
	_, err := savedsearch.New(repo).Create(context.Background(), 7, "Huge query", strings.Repeat("q", 2001), false)
	if !errors.Is(err, savedsearch.ErrQueryTooLong) {
		t.Errorf("err = %v, want ErrQueryTooLong", err)
	}
	if repo.createCalled {
		t.Error("repo.Create should not be called on an over-long query")
	}
}

func TestCreate_AcceptsQueryAtTheLengthCap(t *testing.T) {
	repo := &fakeRepo{createRet: savedsearch.SavedSearch{ID: 1}}
	_, err := savedsearch.New(repo).Create(context.Background(), 7, "At the cap", strings.Repeat("q", 2000), false)
	if err != nil {
		t.Errorf("query at the 2000-char cap: err = %v, want nil", err)
	}
	if !repo.createCalled {
		t.Error("repo.Create should be called for a query at the cap")
	}
}

func TestCreate_EnforcesCap(t *testing.T) {
	repo := &fakeRepo{count: 50} // already at the cap
	_, err := savedsearch.New(repo).Create(context.Background(), 7, "One more", "", false)
	if !errors.Is(err, savedsearch.ErrCapExceeded) {
		t.Errorf("err = %v, want ErrCapExceeded", err)
	}
	if repo.createCalled {
		t.Error("repo.Create should not be called once the cap is reached")
	}
}

func TestCreate_PropagatesDuplicateName(t *testing.T) {
	repo := &fakeRepo{createErr: savedsearch.ErrDuplicateName}
	_, err := savedsearch.New(repo).Create(context.Background(), 7, "Dup", "", false)
	if !errors.Is(err, savedsearch.ErrDuplicateName) {
		t.Errorf("err = %v, want ErrDuplicateName", err)
	}
}

func TestCreate_DerivedFromProfilePassedThrough(t *testing.T) {
	repo := &fakeRepo{createRet: savedsearch.SavedSearch{ID: 1, DerivedFromProfile: true}}
	got, err := savedsearch.New(repo).Create(context.Background(), 7, "My profile", "q=go", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !repo.created.DerivedFromProfile {
		t.Error("repo.Create was not told derivedFromProfile=true")
	}
	if !got.DerivedFromProfile {
		t.Error("returned SavedSearch.DerivedFromProfile = false, want true")
	}
}

func TestCreate_PropagatesProfileSearchExists(t *testing.T) {
	repo := &fakeRepo{createErr: savedsearch.ErrProfileSearchExists}
	_, err := savedsearch.New(repo).Create(context.Background(), 7, "My profile", "", true)
	if !errors.Is(err, savedsearch.ErrProfileSearchExists) {
		t.Errorf("err = %v, want ErrProfileSearchExists", err)
	}
}

func TestUpdate_PartialFields(t *testing.T) {
	repo := &fakeRepo{updateRet: savedsearch.SavedSearch{ID: 5}}
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
	if repo.updated.Name == nil || *repo.updated.Name != "Renamed" {
		t.Errorf("Name param = %v, want trimmed %q", repo.updated.Name, "Renamed")
	}
	if repo.updated.Query != nil {
		t.Error("Query param should be nil (unchanged) when not provided")
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

func TestUpdate_RejectsOverLongQuery(t *testing.T) {
	repo := &fakeRepo{}
	huge := strings.Repeat("q", 2001)
	_, err := savedsearch.New(repo).Update(context.Background(), 7, 5, nil, &huge)
	if !errors.Is(err, savedsearch.ErrQueryTooLong) {
		t.Errorf("err = %v, want ErrQueryTooLong", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called on an over-long query")
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
