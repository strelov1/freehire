package savedsearch_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

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

	getRet    db.SavedSearch
	getErr    error
	getParams db.GetSavedSearchParams

	setRet    db.SavedSearch
	setErrs   []error // consumed one per SetPublicSlug call (nil = success)
	setCalls  int
	setParams []db.SetSavedSearchPublicSlugParams

	clearErr    error
	clearCalled bool
	clearParams db.ClearSavedSearchPublicSlugParams

	boardRet    db.GetPublicBoardBySlugRow
	boardErr    error
	boardSlug   string
	boardCalled bool
}

func (f *fakeRepo) Get(_ context.Context, p db.GetSavedSearchParams) (db.SavedSearch, error) {
	f.getParams = p
	return f.getRet, f.getErr
}

func (f *fakeRepo) SetPublicSlug(_ context.Context, p db.SetSavedSearchPublicSlugParams) (db.SavedSearch, error) {
	f.setParams = append(f.setParams, p)
	i := f.setCalls
	f.setCalls++
	if i < len(f.setErrs) && f.setErrs[i] != nil {
		return db.SavedSearch{}, f.setErrs[i]
	}
	return f.setRet, nil
}

func (f *fakeRepo) ClearPublicSlug(_ context.Context, p db.ClearSavedSearchPublicSlugParams) error {
	f.clearParams, f.clearCalled = p, true
	return f.clearErr
}

func (f *fakeRepo) GetPublicBoard(_ context.Context, slug string) (db.GetPublicBoardBySlugRow, error) {
	f.boardSlug, f.boardCalled = slug, true
	return f.boardRet, f.boardErr
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

func TestShare_MintsReadableSlugFromName(t *testing.T) {
	repo := &fakeRepo{
		getRet: db.SavedSearch{ID: 5, UserID: 7, Name: "Remote Go"}, // private (no slug)
		setRet: db.SavedSearch{ID: 5, PublicSlug: pgtype.Text{String: "remote-go-a3f1", Valid: true}},
	}
	svc := savedsearch.New(repo)

	got, err := svc.Share(context.Background(), 7, 5, "strelov")
	if err != nil {
		t.Fatalf("Share: %v", err)
	}
	if repo.getParams.ID != 5 || repo.getParams.UserID != 7 {
		t.Errorf("Get scope = id %d user %d, want id 5 user 7", repo.getParams.ID, repo.getParams.UserID)
	}
	if repo.setCalls != 1 {
		t.Fatalf("SetPublicSlug calls = %d, want 1", repo.setCalls)
	}
	p := repo.setParams[0]
	if p.ID != 5 || p.UserID != 7 {
		t.Errorf("set scope = id %d user %d, want id 5 user 7", p.ID, p.UserID)
	}
	if !p.PublicSlug.Valid || !strings.HasPrefix(p.PublicSlug.String, "remote-go-") {
		t.Errorf("minted slug = %q, want readable prefix %q", p.PublicSlug.String, "remote-go-")
	}
	if p.PublicSlug.String == "remote-go-" {
		t.Error("minted slug has no random suffix")
	}
	if !p.AuthorLabel.Valid || p.AuthorLabel.String != "strelov" {
		t.Errorf("author label param = %+v, want valid %q", p.AuthorLabel, "strelov")
	}
	if got.PublicSlug.String != "remote-go-a3f1" {
		t.Errorf("returned slug = %q, want the persisted row's", got.PublicSlug.String)
	}
}

func TestShare_KeepsExistingSlugOnReshare(t *testing.T) {
	repo := &fakeRepo{
		getRet: db.SavedSearch{ID: 5, UserID: 7, Name: "Remote Go",
			PublicSlug: pgtype.Text{String: "remote-go-old1", Valid: true}}, // already shared
		setRet: db.SavedSearch{ID: 5, PublicSlug: pgtype.Text{String: "remote-go-old1", Valid: true}},
	}
	_, err := savedsearch.New(repo).Share(context.Background(), 7, 5, "new label")
	if err != nil {
		t.Fatalf("re-share: %v", err)
	}
	if repo.setParams[0].PublicSlug.String != "remote-go-old1" {
		t.Errorf("re-share slug = %q, want existing %q kept", repo.setParams[0].PublicSlug.String, "remote-go-old1")
	}
	if repo.setParams[0].AuthorLabel.String != "new label" {
		t.Errorf("re-share author label = %q, want updated %q", repo.setParams[0].AuthorLabel.String, "new label")
	}
}

func TestShare_EmptyLabelIsAnonymous(t *testing.T) {
	repo := &fakeRepo{getRet: db.SavedSearch{ID: 5, UserID: 7, Name: "X"}, setRet: db.SavedSearch{ID: 5}}
	if _, err := savedsearch.New(repo).Share(context.Background(), 7, 5, "   "); err != nil {
		t.Fatalf("Share: %v", err)
	}
	if repo.setParams[0].AuthorLabel.Valid {
		t.Errorf("author label = %+v, want NULL (anonymous) for blank input", repo.setParams[0].AuthorLabel)
	}
}

func TestShare_RejectsOverLongLabel(t *testing.T) {
	repo := &fakeRepo{getRet: db.SavedSearch{ID: 5, UserID: 7, Name: "X"}}
	_, err := savedsearch.New(repo).Share(context.Background(), 7, 5, strings.Repeat("x", 61))
	if !errors.Is(err, savedsearch.ErrInvalidAuthorLabel) {
		t.Errorf("err = %v, want ErrInvalidAuthorLabel", err)
	}
	if repo.setCalls != 0 {
		t.Error("SetPublicSlug should not be called on an invalid label")
	}
}

func TestShare_NotOwned(t *testing.T) {
	repo := &fakeRepo{getErr: savedsearch.ErrNotFound}
	_, err := savedsearch.New(repo).Share(context.Background(), 7, 999, "")
	if !errors.Is(err, savedsearch.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if repo.setCalls != 0 {
		t.Error("SetPublicSlug should not be called when the set is not owned")
	}
}

func TestShare_RetriesOnSlugCollision(t *testing.T) {
	repo := &fakeRepo{
		getRet:  db.SavedSearch{ID: 5, UserID: 7, Name: "Remote Go"},
		setErrs: []error{savedsearch.ErrSlugTaken}, // first attempt collides, second succeeds
		setRet:  db.SavedSearch{ID: 5, PublicSlug: pgtype.Text{String: "remote-go-b2c3", Valid: true}},
	}
	_, err := savedsearch.New(repo).Share(context.Background(), 7, 5, "")
	if err != nil {
		t.Fatalf("Share with one collision: %v", err)
	}
	if repo.setCalls != 2 {
		t.Errorf("SetPublicSlug calls = %d, want 2 (retry after collision)", repo.setCalls)
	}
}

func TestUnshare_ScopedToOwner(t *testing.T) {
	repo := &fakeRepo{}
	if err := savedsearch.New(repo).Unshare(context.Background(), 7, 5); err != nil {
		t.Fatalf("Unshare: %v", err)
	}
	if !repo.clearCalled || repo.clearParams.ID != 5 || repo.clearParams.UserID != 7 {
		t.Errorf("clear scope = id %d user %d, want id 5 user 7", repo.clearParams.ID, repo.clearParams.UserID)
	}
}

func TestUnshare_NotFound(t *testing.T) {
	repo := &fakeRepo{clearErr: savedsearch.ErrNotFound}
	if err := savedsearch.New(repo).Unshare(context.Background(), 7, 999); !errors.Is(err, savedsearch.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGetPublicBoard_ReturnsBoard(t *testing.T) {
	repo := &fakeRepo{boardRet: db.GetPublicBoardBySlugRow{Name: "Remote Go", Query: "q=go"}}
	got, err := savedsearch.New(repo).GetPublicBoard(context.Background(), "remote-go-a3f1")
	if err != nil {
		t.Fatalf("GetPublicBoard: %v", err)
	}
	if repo.boardSlug != "remote-go-a3f1" {
		t.Errorf("looked up slug = %q, want %q", repo.boardSlug, "remote-go-a3f1")
	}
	if got.Name != "Remote Go" || got.Query != "q=go" {
		t.Errorf("board = %+v, want name/query carried through", got)
	}
}

func TestGetPublicBoard_NotFound(t *testing.T) {
	repo := &fakeRepo{boardErr: savedsearch.ErrNotFound}
	if _, err := savedsearch.New(repo).GetPublicBoard(context.Background(), "nope"); !errors.Is(err, savedsearch.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
