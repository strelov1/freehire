package joblists_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/application/joblists"
)

type createArgs struct {
	UserID      int64
	Name        string
	Description string
}

type updateArgs struct {
	ID          int64
	UserID      int64
	Name        *string
	Description *string
}

type getArgs struct {
	ID     int64
	UserID int64
}

type setArgs struct {
	ID         int64
	UserID     int64
	PublicSlug string
}

type deleteArgs struct {
	ID     int64
	UserID int64
}

type clearArgs struct {
	ID     int64
	UserID int64
}

type addJobArgs struct {
	ID    int64
	JobID int64
}

type removeJobArgs struct {
	ID    int64
	JobID int64
}

type jobIDBySlugCall struct {
	slug string
}

// fakeRepo records the params it is handed and returns canned rows/errors, so the
// service tests run without a database (the savedsearch_test.go precedent).
type fakeRepo struct {
	count    int64
	countErr error

	created      createArgs
	createCalled bool
	createErr    error
	createRet    joblists.JobList

	updated      updateArgs
	updateCalled bool
	updateErr    error
	updateRet    joblists.JobList

	deleted      deleteArgs
	deleteCalled bool
	deleteErr    error

	listRet []joblists.JobList

	getRet    joblists.JobList
	getErr    error
	getParams getArgs
	getCalled bool

	setRet    joblists.JobList
	setErrs   []error // consumed one per SetPublicSlug call (nil = success)
	setCalls  int
	setParams []setArgs

	clearErr    error
	clearCalled bool
	clearParams clearArgs

	itemCount    int64
	itemCountErr error

	hasItem    bool
	hasItemErr error

	addJobErr    error
	addJobCalled bool
	addJobParams addJobArgs

	removeJobErr    error
	removeJobCalled bool
	removeJobParams removeJobArgs

	jobIDBySlugRet   int64
	jobIDBySlugErr   error
	jobIDBySlugCalls []jobIDBySlugCall

	publicRet joblists.PublicJobList
	publicErr error
	publicArg string

	membershipRet    []joblists.ListMembership
	membershipErr    error
	membershipUserID int64
	membershipJobID  int64
}

func (f *fakeRepo) List(_ context.Context, _ int64) ([]joblists.JobList, error) {
	return f.listRet, nil
}

func (f *fakeRepo) Count(_ context.Context, _ int64) (int64, error) {
	return f.count, f.countErr
}

func (f *fakeRepo) Create(_ context.Context, userID int64, name, description string) (joblists.JobList, error) {
	f.created, f.createCalled = createArgs{UserID: userID, Name: name, Description: description}, true
	return f.createRet, f.createErr
}

func (f *fakeRepo) Update(_ context.Context, id, userID int64, name, description *string) (joblists.JobList, error) {
	f.updated, f.updateCalled = updateArgs{ID: id, UserID: userID, Name: name, Description: description}, true
	return f.updateRet, f.updateErr
}

func (f *fakeRepo) Delete(_ context.Context, id, userID int64) error {
	f.deleted, f.deleteCalled = deleteArgs{ID: id, UserID: userID}, true
	return f.deleteErr
}

func (f *fakeRepo) Get(_ context.Context, id, userID int64) (joblists.JobList, error) {
	f.getParams, f.getCalled = getArgs{ID: id, UserID: userID}, true
	return f.getRet, f.getErr
}

func (f *fakeRepo) SetPublicSlug(_ context.Context, id, userID int64, publicSlug string) (joblists.JobList, error) {
	f.setParams = append(f.setParams, setArgs{ID: id, UserID: userID, PublicSlug: publicSlug})
	i := f.setCalls
	f.setCalls++
	if i < len(f.setErrs) && f.setErrs[i] != nil {
		return joblists.JobList{}, f.setErrs[i]
	}
	return f.setRet, nil
}

func (f *fakeRepo) ClearPublicSlug(_ context.Context, id, userID int64) error {
	f.clearParams, f.clearCalled = clearArgs{ID: id, UserID: userID}, true
	return f.clearErr
}

func (f *fakeRepo) CountItems(_ context.Context, _ int64) (int64, error) {
	return f.itemCount, f.itemCountErr
}

func (f *fakeRepo) HasItem(_ context.Context, _, _ int64) (bool, error) {
	return f.hasItem, f.hasItemErr
}

func (f *fakeRepo) AddJob(_ context.Context, id, jobID int64) error {
	f.addJobParams, f.addJobCalled = addJobArgs{ID: id, JobID: jobID}, true
	return f.addJobErr
}

func (f *fakeRepo) RemoveJob(_ context.Context, id, jobID int64) error {
	f.removeJobParams, f.removeJobCalled = removeJobArgs{ID: id, JobID: jobID}, true
	return f.removeJobErr
}

func (f *fakeRepo) JobIDBySlug(_ context.Context, slug string) (int64, error) {
	f.jobIDBySlugCalls = append(f.jobIDBySlugCalls, jobIDBySlugCall{slug: slug})
	return f.jobIDBySlugRet, f.jobIDBySlugErr
}

func (f *fakeRepo) GetPublicList(_ context.Context, slug string) (joblists.PublicJobList, error) {
	f.publicArg = slug
	return f.publicRet, f.publicErr
}

func (f *fakeRepo) MembershipForJob(_ context.Context, userID, jobID int64) ([]joblists.ListMembership, error) {
	f.membershipUserID, f.membershipJobID = userID, jobID
	return f.membershipRet, f.membershipErr
}

func TestCreate_PersistsWithOwnerAndTrimmedName(t *testing.T) {
	repo := &fakeRepo{createRet: joblists.JobList{ID: 1}}
	svc := joblists.New(repo)

	_, err := svc.Create(context.Background(), 7, "  Backend jobs  ", "my shortlist")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("repo.Create was not called")
	}
	if repo.created.UserID != 7 {
		t.Errorf("UserID = %d, want 7", repo.created.UserID)
	}
	if repo.created.Name != "Backend jobs" {
		t.Errorf("Name = %q, want trimmed %q", repo.created.Name, "Backend jobs")
	}
	if repo.created.Description != "my shortlist" {
		t.Errorf("Description = %q, not carried through", repo.created.Description)
	}
}

func TestCreate_EmptyDescriptionAllowed(t *testing.T) {
	repo := &fakeRepo{createRet: joblists.JobList{ID: 1}}
	_, err := joblists.New(repo).Create(context.Background(), 7, "No description", "")
	if err != nil {
		t.Fatalf("Create with empty description: %v", err)
	}
	if repo.created.Description != "" {
		t.Errorf("Description = %q, want empty", repo.created.Description)
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
			_, err := joblists.New(repo).Create(context.Background(), 7, tc.in, "")
			if !errors.Is(err, joblists.ErrInvalidName) {
				t.Errorf("err = %v, want ErrInvalidName", err)
			}
			if repo.createCalled {
				t.Error("repo.Create should not be called on an invalid name")
			}
		})
	}
}

func TestCreate_RejectsOverLongDescription(t *testing.T) {
	repo := &fakeRepo{}
	_, err := joblists.New(repo).Create(context.Background(), 7, "Huge description", strings.Repeat("d", 2001))
	if !errors.Is(err, joblists.ErrInvalidDescription) {
		t.Errorf("err = %v, want ErrInvalidDescription", err)
	}
	if repo.createCalled {
		t.Error("repo.Create should not be called on an over-long description")
	}
}

func TestCreate_EnforcesCap(t *testing.T) {
	repo := &fakeRepo{count: 50} // already at the cap
	_, err := joblists.New(repo).Create(context.Background(), 7, "One more", "")
	if !errors.Is(err, joblists.ErrCapExceeded) {
		t.Errorf("err = %v, want ErrCapExceeded", err)
	}
	if repo.createCalled {
		t.Error("repo.Create should not be called once the cap is reached")
	}
}

func TestCreate_PropagatesDuplicateName(t *testing.T) {
	repo := &fakeRepo{createErr: joblists.ErrDuplicateName}
	_, err := joblists.New(repo).Create(context.Background(), 7, "Dup", "")
	if !errors.Is(err, joblists.ErrDuplicateName) {
		t.Errorf("err = %v, want ErrDuplicateName", err)
	}
}

func TestUpdate_PartialFields(t *testing.T) {
	repo := &fakeRepo{updateRet: joblists.JobList{ID: 5}}
	svc := joblists.New(repo)

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
	if repo.updated.Description != nil {
		t.Error("Description param should be nil (unchanged) when not provided")
	}
}

func TestUpdate_RejectsInvalidName(t *testing.T) {
	repo := &fakeRepo{}
	blank := "  "
	_, err := joblists.New(repo).Update(context.Background(), 7, 5, &blank, nil)
	if !errors.Is(err, joblists.ErrInvalidName) {
		t.Errorf("err = %v, want ErrInvalidName", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called on an invalid name")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	repo := &fakeRepo{updateErr: joblists.ErrNotFound}
	desc := "x"
	_, err := joblists.New(repo).Update(context.Background(), 7, 999, nil, &desc)
	if !errors.Is(err, joblists.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDelete_ScopedToOwner(t *testing.T) {
	repo := &fakeRepo{}
	err := joblists.New(repo).Delete(context.Background(), 7, 5)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if repo.deleted.ID != 5 || repo.deleted.UserID != 7 {
		t.Errorf("delete scope = id %d user %d, want id 5 user 7", repo.deleted.ID, repo.deleted.UserID)
	}
}

func TestDelete_NotFound(t *testing.T) {
	repo := &fakeRepo{deleteErr: joblists.ErrNotFound}
	err := joblists.New(repo).Delete(context.Background(), 7, 999)
	if !errors.Is(err, joblists.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestAddJob_ChecksOwnershipThenAdds(t *testing.T) {
	repo := &fakeRepo{getRet: joblists.JobList{ID: 5}, jobIDBySlugRet: 42}
	err := joblists.New(repo).AddJob(context.Background(), 7, 5, "go-dev-acme-1a2b")
	if err != nil {
		t.Fatalf("AddJob: %v", err)
	}
	if repo.getParams.ID != 5 || repo.getParams.UserID != 7 {
		t.Errorf("ownership check scope = id %d user %d, want id 5 user 7", repo.getParams.ID, repo.getParams.UserID)
	}
	if len(repo.jobIDBySlugCalls) != 1 || repo.jobIDBySlugCalls[0].slug != "go-dev-acme-1a2b" {
		t.Errorf("JobIDBySlug calls = %+v, want one call with the given slug", repo.jobIDBySlugCalls)
	}
	if !repo.addJobCalled || repo.addJobParams.ID != 5 || repo.addJobParams.JobID != 42 {
		t.Errorf("AddJob params = %+v, want list 5 job 42", repo.addJobParams)
	}
}

func TestAddJob_NotOwned(t *testing.T) {
	repo := &fakeRepo{getErr: joblists.ErrNotFound}
	err := joblists.New(repo).AddJob(context.Background(), 7, 999, "go-dev-acme-1a2b")
	if !errors.Is(err, joblists.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if repo.addJobCalled {
		t.Error("repo.AddJob should not be called when the list is not owned")
	}
}

func TestAddJob_RejectsPastPerListCap(t *testing.T) {
	repo := &fakeRepo{getRet: joblists.JobList{ID: 5}, jobIDBySlugRet: 42, hasItem: false, itemCount: 200}
	err := joblists.New(repo).AddJob(context.Background(), 7, 5, "go-dev-acme-1a2b")
	if !errors.Is(err, joblists.ErrListFull) {
		t.Errorf("err = %v, want ErrListFull", err)
	}
	if repo.addJobCalled {
		t.Error("repo.AddJob should not be called once the per-list cap is reached")
	}
}

func TestAddJob_ExemptFromCapWhenAlreadyAMember(t *testing.T) {
	repo := &fakeRepo{getRet: joblists.JobList{ID: 5}, jobIDBySlugRet: 42, hasItem: true, itemCount: 200}
	err := joblists.New(repo).AddJob(context.Background(), 7, 5, "go-dev-acme-1a2b")
	if err != nil {
		t.Fatalf("AddJob for an existing member at the cap: %v, want nil (idempotent)", err)
	}
	if !repo.addJobCalled {
		t.Error("repo.AddJob should still be called for an idempotent re-add")
	}
}

func TestAddJob_RejectsUnknownJob(t *testing.T) {
	repo := &fakeRepo{getRet: joblists.JobList{ID: 5}, jobIDBySlugErr: joblists.ErrJobNotFound}
	err := joblists.New(repo).AddJob(context.Background(), 7, 5, "nonexistent")
	if !errors.Is(err, joblists.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
	if repo.addJobCalled {
		t.Error("repo.AddJob should not be called for an unknown job")
	}
}

func TestRemoveJob_ChecksOwnershipThenRemoves(t *testing.T) {
	repo := &fakeRepo{getRet: joblists.JobList{ID: 5}, jobIDBySlugRet: 42}
	err := joblists.New(repo).RemoveJob(context.Background(), 7, 5, "go-dev-acme-1a2b")
	if err != nil {
		t.Fatalf("RemoveJob: %v", err)
	}
	if !repo.removeJobCalled || repo.removeJobParams.ID != 5 || repo.removeJobParams.JobID != 42 {
		t.Errorf("RemoveJob params = %+v, want list 5 job 42", repo.removeJobParams)
	}
}

func TestRemoveJob_NotOwned(t *testing.T) {
	repo := &fakeRepo{getErr: joblists.ErrNotFound}
	err := joblists.New(repo).RemoveJob(context.Background(), 7, 999, "go-dev-acme-1a2b")
	if !errors.Is(err, joblists.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if repo.removeJobCalled {
		t.Error("repo.RemoveJob should not be called when the list is not owned")
	}
}

func TestRemoveJob_UnknownSlugIsIdempotent(t *testing.T) {
	repo := &fakeRepo{getRet: joblists.JobList{ID: 5}, jobIDBySlugErr: joblists.ErrJobNotFound}
	err := joblists.New(repo).RemoveJob(context.Background(), 7, 5, "nonexistent")
	if err != nil {
		t.Fatalf("RemoveJob with an unknown slug: %v, want nil (idempotent)", err)
	}
	if repo.removeJobCalled {
		t.Error("repo.RemoveJob should not be called for a slug that resolves to no job")
	}
}

func TestShare_MintsReadableSlugFromName(t *testing.T) {
	repo := &fakeRepo{
		getRet: joblists.JobList{ID: 5, Name: "Backend jobs"}, // private (no slug)
		setRet: joblists.JobList{ID: 5, PublicSlug: "backend-jobs-a3f1"},
	}
	svc := joblists.New(repo)

	got, err := svc.Share(context.Background(), 7, 5)
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
	if p.PublicSlug == "" || !strings.HasPrefix(p.PublicSlug, "backend-jobs-") {
		t.Errorf("minted slug = %q, want readable prefix %q", p.PublicSlug, "backend-jobs-")
	}
	if got.PublicSlug != "backend-jobs-a3f1" {
		t.Errorf("returned slug = %q, want the persisted row's", got.PublicSlug)
	}
}

func TestShare_KeepsExistingSlugOnReshare(t *testing.T) {
	repo := &fakeRepo{
		getRet: joblists.JobList{ID: 5, Name: "Backend jobs", PublicSlug: "backend-jobs-old1"}, // already shared
		setRet: joblists.JobList{ID: 5, PublicSlug: "backend-jobs-old1"},
	}
	_, err := joblists.New(repo).Share(context.Background(), 7, 5)
	if err != nil {
		t.Fatalf("re-share: %v", err)
	}
	if repo.setParams[0].PublicSlug != "backend-jobs-old1" {
		t.Errorf("re-share slug = %q, want existing %q kept", repo.setParams[0].PublicSlug, "backend-jobs-old1")
	}
}

func TestShare_NotOwned(t *testing.T) {
	repo := &fakeRepo{getErr: joblists.ErrNotFound}
	_, err := joblists.New(repo).Share(context.Background(), 7, 999)
	if !errors.Is(err, joblists.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if repo.setCalls != 0 {
		t.Error("SetPublicSlug should not be called when the list is not owned")
	}
}

func TestShare_RetriesOnSlugCollision(t *testing.T) {
	repo := &fakeRepo{
		getRet:  joblists.JobList{ID: 5, Name: "Backend jobs"},
		setErrs: []error{joblists.ErrSlugTaken}, // first attempt collides, second succeeds
		setRet:  joblists.JobList{ID: 5, PublicSlug: "backend-jobs-b2c3"},
	}
	_, err := joblists.New(repo).Share(context.Background(), 7, 5)
	if err != nil {
		t.Fatalf("Share with one collision: %v", err)
	}
	if repo.setCalls != 2 {
		t.Errorf("SetPublicSlug calls = %d, want 2 (retry after collision)", repo.setCalls)
	}
}

func TestUnshare_ScopedToOwner(t *testing.T) {
	repo := &fakeRepo{}
	if err := joblists.New(repo).Unshare(context.Background(), 7, 5); err != nil {
		t.Fatalf("Unshare: %v", err)
	}
	if !repo.clearCalled || repo.clearParams.ID != 5 || repo.clearParams.UserID != 7 {
		t.Errorf("clear scope = id %d user %d, want id 5 user 7", repo.clearParams.ID, repo.clearParams.UserID)
	}
}

func TestUnshare_NotFound(t *testing.T) {
	repo := &fakeRepo{clearErr: joblists.ErrNotFound}
	if err := joblists.New(repo).Unshare(context.Background(), 7, 999); !errors.Is(err, joblists.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGetPublicList_ReturnsList(t *testing.T) {
	repo := &fakeRepo{publicRet: joblists.PublicJobList{Name: "Backend jobs", Description: "shortlist"}}
	got, err := joblists.New(repo).GetPublicList(context.Background(), "backend-jobs-a3f1")
	if err != nil {
		t.Fatalf("GetPublicList: %v", err)
	}
	if repo.publicArg != "backend-jobs-a3f1" {
		t.Errorf("looked up slug = %q, want %q", repo.publicArg, "backend-jobs-a3f1")
	}
	if got.Name != "Backend jobs" || got.Description != "shortlist" {
		t.Errorf("list = %+v, want name/description carried through", got)
	}
}

func TestGetPublicList_NotFound(t *testing.T) {
	repo := &fakeRepo{publicErr: joblists.ErrNotFound}
	if _, err := joblists.New(repo).GetPublicList(context.Background(), "nope"); !errors.Is(err, joblists.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListMembership_ResolvesSlugThenReadsMembership(t *testing.T) {
	repo := &fakeRepo{
		jobIDBySlugRet: 42,
		membershipRet: []joblists.ListMembership{
			{ID: 1, Name: "Backend jobs", InList: true},
			{ID: 2, Name: "Later", InList: false},
		},
	}
	got, err := joblists.New(repo).ListMembership(context.Background(), 7, "go-dev-acme-1a2b")
	if err != nil {
		t.Fatalf("ListMembership: %v", err)
	}
	if repo.membershipUserID != 7 || repo.membershipJobID != 42 {
		t.Errorf("MembershipForJob scope = user %d job %d, want user 7 job 42", repo.membershipUserID, repo.membershipJobID)
	}
	if len(got) != 2 || !got[0].InList || got[1].InList {
		t.Errorf("membership = %+v, want the fake's rows carried through", got)
	}
}

func TestListMembership_UnknownSlug(t *testing.T) {
	repo := &fakeRepo{jobIDBySlugErr: joblists.ErrJobNotFound}
	_, err := joblists.New(repo).ListMembership(context.Background(), 7, "nonexistent")
	if !errors.Is(err, joblists.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}
