package submission_test

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/submission"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeRepo records the domain inputs it is handed and returns canned rows, so the service
// tests run without a database (the moderation_test.go precedent).
type fakeRepo struct {
	created         moderation.CreateInput
	createSubmitter int64
	createCalled    bool
	createErr       error
	createRet       submission.Submission

	getRet submission.Submission
	getErr error

	approveID       int64
	approveReviewer int64
	approveJobID    int64
	approveCalled   bool
	approveErr      error
	approveRet      submission.Submission

	rejectID       int64
	rejectReviewer int64
	rejectReason   string
	rejectCalled   bool
	rejectErr      error
	rejectRet      submission.Submission

	isHostBlockedHost   string
	isHostBlockedCalled bool
	isHostBlockedRet    bool
	isHostBlockedErr    error

	blockID       int64
	blockHost     string
	blockReviewer int64
	blockReason   string
	blockCalled   bool
	blockErr      error
	blockRet      submission.Submission
}

func (f *fakeRepo) Create(_ context.Context, submittedBy int64, in moderation.CreateInput) (submission.Submission, error) {
	f.created, f.createSubmitter, f.createCalled = in, submittedBy, true
	return f.createRet, f.createErr
}

func (f *fakeRepo) Get(_ context.Context, _ int64) (submission.Submission, error) {
	return f.getRet, f.getErr
}

func (f *fakeRepo) ListPending(_ context.Context) ([]submission.PendingSubmission, error) {
	return nil, nil
}

func (f *fakeRepo) ListByUser(_ context.Context, _ int64) ([]submission.UserSubmission, error) {
	return nil, nil
}

func (f *fakeRepo) MarkApproved(_ context.Context, id, reviewerID, jobID int64) (submission.Submission, error) {
	f.approveID, f.approveReviewer, f.approveJobID, f.approveCalled = id, reviewerID, jobID, true
	return f.approveRet, f.approveErr
}

func (f *fakeRepo) MarkRejected(_ context.Context, id, reviewerID int64, reason string) (submission.Submission, error) {
	f.rejectID, f.rejectReviewer, f.rejectReason, f.rejectCalled = id, reviewerID, reason, true
	return f.rejectRet, f.rejectErr
}

func (f *fakeRepo) IsHostBlocked(_ context.Context, host string) (bool, error) {
	f.isHostBlockedHost, f.isHostBlockedCalled = host, true
	return f.isHostBlockedRet, f.isHostBlockedErr
}

func (f *fakeRepo) RejectAndBlockHost(_ context.Context, id int64, host string, reviewerID int64, reason string) (submission.Submission, error) {
	f.blockID, f.blockHost, f.blockReviewer, f.blockReason, f.blockCalled = id, host, reviewerID, reason, true
	return f.blockRet, f.blockErr
}

// fakeMinter stands in for moderation.Service: it records the approve-time mint call.
type fakeMinter struct {
	actorID int64
	in      moderation.CreateInput
	called  bool
	ret     job.Job
	err     error
}

func (m *fakeMinter) Create(_ context.Context, actorID int64, in moderation.CreateInput) (job.Job, job.Extras, error) {
	m.actorID, m.in, m.called = actorID, in, true
	return m.ret, job.Extras{}, m.err
}

// mustJob hydrates a db row into the aggregate for the minted-job fixture.
func mustJob(t *testing.T, r db.Job) job.Job {
	t.Helper()
	j, _, err := job.FromRow(r)
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	return j
}

func validInput() moderation.CreateInput {
	return moderation.CreateInput{
		URL:      "https://acme.example/jobs/1",
		Title:    "Senior Go Developer",
		Company:  "Acme",
		Location: "Berlin",
		Remote:   true,
	}
}

func TestSubmit_PersistsPendingWithOwner(t *testing.T) {
	repo := &fakeRepo{createRet: submission.Submission{ID: 1, Status: "pending"}}
	svc := submission.New(repo, &fakeMinter{})

	_, err := svc.Submit(context.Background(), 7, validInput())
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("repo.Create was not called")
	}
	if repo.createSubmitter != 7 {
		t.Errorf("submittedBy = %d, want 7", repo.createSubmitter)
	}
	got := repo.created
	if got.URL != "https://acme.example/jobs/1" || got.Title != "Senior Go Developer" || got.Company != "Acme" {
		t.Errorf("content not carried through: %+v", got)
	}
	if got.Location != "Berlin" || !got.Remote {
		t.Errorf("optional fields not carried: location=%q remote=%v", got.Location, got.Remote)
	}
}

func TestSubmit_ValidatesBeforePersist(t *testing.T) {
	cases := []struct {
		name string
		in   moderation.CreateInput
	}{
		{"missing url", moderation.CreateInput{Title: "T", Company: "C"}},
		{"missing title", moderation.CreateInput{URL: "https://x/1", Company: "C"}},
		{"missing company", moderation.CreateInput{URL: "https://x/1", Title: "T"}},
		{"non-http url", moderation.CreateInput{URL: "ftp://x/1", Title: "T", Company: "C"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, tc.in)
			if !errors.Is(err, moderation.ErrInvalid) {
				t.Errorf("err = %v, want moderation.ErrInvalid", err)
			}
			if repo.createCalled {
				t.Error("repo.Create should not be called on invalid input")
			}
		})
	}
}

func TestSubmit_PropagatesDuplicatePending(t *testing.T) {
	repo := &fakeRepo{createErr: submission.ErrDuplicatePending}
	_, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, validInput())
	if !errors.Is(err, submission.ErrDuplicatePending) {
		t.Errorf("err = %v, want ErrDuplicatePending", err)
	}
}

func TestApprove_MintsUnderSubmitterAndMarks(t *testing.T) {
	sub := submission.Submission{ID: 5, SubmittedBy: 7, Status: "pending", URL: "https://x/1", Source: "workatastartup", Title: "Dev", Company: "Acme", Location: "Berlin", Remote: true, Description: "Build <b>it</b>"}
	repo := &fakeRepo{getRet: sub, approveRet: submission.Submission{ID: 5, Status: "approved"}}
	minter := &fakeMinter{ret: mustJob(t, db.Job{ID: 99})}
	svc := submission.New(repo, minter)

	_, err := svc.Approve(context.Background(), 3, 5)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if !minter.called {
		t.Fatal("minter.Create was not called")
	}
	if minter.actorID != 7 {
		t.Errorf("mint actorID = %d, want 7 (the submitter, as job author)", minter.actorID)
	}
	// Every content field must travel from the submission to the mint input — especially
	// Description, the one field the minter sanitizes, and Source, which the minter defaults.
	if minter.in.URL != "https://x/1" || minter.in.Source != "workatastartup" ||
		minter.in.Company != "Acme" || !minter.in.Remote || minter.in.Description != "Build <b>it</b>" {
		t.Errorf("mint input not built from submission: %+v", minter.in)
	}
	if !repo.approveCalled {
		t.Fatal("repo.MarkApproved was not called")
	}
	if repo.approveID != 5 || repo.approveReviewer != 3 || repo.approveJobID != 99 {
		t.Errorf("approve params = id=%d reviewer=%d job=%d, want id=5 reviewer=3 job=99", repo.approveID, repo.approveReviewer, repo.approveJobID)
	}
}

func TestApprove_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: submission.ErrSubmissionNotFound}
	minter := &fakeMinter{}
	_, err := submission.New(repo, minter).Approve(context.Background(), 3, 5)
	if !errors.Is(err, submission.ErrSubmissionNotFound) {
		t.Errorf("err = %v, want ErrSubmissionNotFound", err)
	}
	if minter.called {
		t.Error("minter.Create should not be called when the submission is missing")
	}
}

func TestApprove_AlreadyDecided(t *testing.T) {
	repo := &fakeRepo{getRet: submission.Submission{ID: 5, Status: "approved"}}
	minter := &fakeMinter{}
	_, err := submission.New(repo, minter).Approve(context.Background(), 3, 5)
	if !errors.Is(err, submission.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if minter.called || repo.approveCalled {
		t.Error("a decided submission must not be minted or re-marked")
	}
}

func TestReject_MarksWithReason(t *testing.T) {
	repo := &fakeRepo{getRet: submission.Submission{ID: 5, Status: "pending"}, rejectRet: submission.Submission{Status: "rejected"}}
	minter := &fakeMinter{}
	_, err := submission.New(repo, minter).Reject(context.Background(), 3, 5, "duplicate", false)
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if !repo.rejectCalled {
		t.Fatal("repo.MarkRejected was not called")
	}
	if repo.rejectID != 5 || repo.rejectReviewer != 3 || repo.rejectReason != "duplicate" {
		t.Errorf("reject params = id=%d reviewer=%d reason=%q, want id=5 reviewer=3 reason=duplicate", repo.rejectID, repo.rejectReviewer, repo.rejectReason)
	}
	if minter.called {
		t.Error("reject must not mint a job")
	}
	if repo.blockCalled {
		t.Error("a plain reject must not touch the blocklist")
	}
}

func TestReject_AlreadyDecided(t *testing.T) {
	repo := &fakeRepo{getRet: submission.Submission{ID: 5, Status: "rejected"}}
	_, err := submission.New(repo, &fakeMinter{}).Reject(context.Background(), 3, 5, "", false)
	if !errors.Is(err, submission.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if repo.rejectCalled {
		t.Error("a decided submission must not be re-marked")
	}
}

func TestReject_AlreadyDecided_BlockDomainNotAttempted(t *testing.T) {
	repo := &fakeRepo{getRet: submission.Submission{ID: 5, Status: "approved"}}
	_, err := submission.New(repo, &fakeMinter{}).Reject(context.Background(), 3, 5, "", true)
	if !errors.Is(err, submission.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if repo.blockCalled {
		t.Error("a decided submission must not trigger a blocklist write")
	}
}

func TestReject_BlockDomain_UsesNormalizedHostOfSubmissionURL(t *testing.T) {
	repo := &fakeRepo{
		getRet:   submission.Submission{ID: 5, Status: "pending", URL: "https://WWW.Gridnaut.Site/jobs/1"},
		blockRet: submission.Submission{ID: 5, Status: "rejected"},
	}
	_, err := submission.New(repo, &fakeMinter{}).Reject(context.Background(), 3, 5, "referral spam", true)
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if !repo.blockCalled {
		t.Fatal("repo.RejectAndBlockHost was not called")
	}
	if repo.blockID != 5 || repo.blockReviewer != 3 || repo.blockReason != "referral spam" {
		t.Errorf("block params = id=%d reviewer=%d reason=%q, want id=5 reviewer=3 reason=%q", repo.blockID, repo.blockReviewer, repo.blockReason, "referral spam")
	}
	if repo.blockHost != "gridnaut.site" {
		t.Errorf("blockHost = %q, want normalized host %q", repo.blockHost, "gridnaut.site")
	}
	if repo.rejectCalled {
		t.Error("block_domain must use RejectAndBlockHost, not the plain MarkRejected path")
	}
}

func TestSubmit_RefusesBlockedHost(t *testing.T) {
	repo := &fakeRepo{isHostBlockedRet: true}
	_, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, validInput())
	if !errors.Is(err, submission.ErrBlockedDomain) {
		t.Errorf("err = %v, want ErrBlockedDomain", err)
	}
	if repo.createCalled {
		t.Error("a blocked host must not be persisted")
	}
}

func TestSubmit_ChecksNormalizedHost(t *testing.T) {
	repo := &fakeRepo{}
	in := validInput()
	in.URL = "https://WWW.Acme.Example/jobs/1"
	if _, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, in); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if repo.isHostBlockedHost != "acme.example" {
		t.Errorf("isHostBlockedHost = %q, want normalized host %q", repo.isHostBlockedHost, "acme.example")
	}
}

func TestSubmit_AllowsNonBlockedHost(t *testing.T) {
	repo := &fakeRepo{isHostBlockedRet: false, createRet: submission.Submission{ID: 1, Status: "pending"}}
	_, err := submission.New(repo, &fakeMinter{}).Submit(context.Background(), 7, validInput())
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !repo.createCalled {
		t.Error("a non-blocked host must be persisted as usual")
	}
}
