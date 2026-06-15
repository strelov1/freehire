package submission_test

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/submission"
)

// fakeRepo records the params it is handed and returns canned rows, so the service tests
// run without a database (the moderation_test.go precedent).
type fakeRepo struct {
	created      db.CreateSubmissionParams
	createCalled bool
	createErr    error
	createRet    db.JobSubmission

	getRet db.JobSubmission
	getErr error

	approved      db.MarkSubmissionApprovedParams
	approveCalled bool
	approveErr    error
	approveRet    db.JobSubmission

	rejected     db.MarkSubmissionRejectedParams
	rejectCalled bool
	rejectErr    error
	rejectRet    db.JobSubmission
}

func (f *fakeRepo) Create(_ context.Context, p db.CreateSubmissionParams) (db.JobSubmission, error) {
	f.created, f.createCalled = p, true
	return f.createRet, f.createErr
}

func (f *fakeRepo) Get(_ context.Context, _ int64) (db.JobSubmission, error) {
	return f.getRet, f.getErr
}

func (f *fakeRepo) ListPending(_ context.Context) ([]db.ListPendingSubmissionsRow, error) {
	return nil, nil
}

func (f *fakeRepo) ListByUser(_ context.Context, _ int64) ([]db.JobSubmission, error) {
	return nil, nil
}

func (f *fakeRepo) MarkApproved(_ context.Context, p db.MarkSubmissionApprovedParams) (db.JobSubmission, error) {
	f.approved, f.approveCalled = p, true
	return f.approveRet, f.approveErr
}

func (f *fakeRepo) MarkRejected(_ context.Context, p db.MarkSubmissionRejectedParams) (db.JobSubmission, error) {
	f.rejected, f.rejectCalled = p, true
	return f.rejectRet, f.rejectErr
}

// fakeMinter stands in for moderation.Service: it records the approve-time mint call.
type fakeMinter struct {
	actorID int64
	in      moderation.CreateInput
	called  bool
	ret     db.Job
	err     error
}

func (m *fakeMinter) Create(_ context.Context, actorID int64, in moderation.CreateInput) (db.Job, error) {
	m.actorID, m.in, m.called = actorID, in, true
	return m.ret, m.err
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
	repo := &fakeRepo{createRet: db.JobSubmission{ID: 1, Status: "pending"}}
	svc := submission.New(repo, &fakeMinter{})

	_, err := svc.Submit(context.Background(), 7, validInput())
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("repo.Create was not called")
	}
	got := repo.created
	if got.SubmittedBy != 7 {
		t.Errorf("SubmittedBy = %d, want 7", got.SubmittedBy)
	}
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
	sub := db.JobSubmission{ID: 5, SubmittedBy: 7, Status: "pending", URL: "https://x/1", Source: "workatastartup", Title: "Dev", Company: "Acme", Location: "Berlin", Remote: true, Description: "Build <b>it</b>"}
	repo := &fakeRepo{getRet: sub, approveRet: db.JobSubmission{ID: 5, Status: "approved"}}
	minter := &fakeMinter{ret: db.Job{ID: 99}}
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
	if repo.approved.ID != 5 || repo.approved.ReviewedBy != 3 || repo.approved.JobID != 99 {
		t.Errorf("approve params = %+v, want id=5 reviewer=3 job=99", repo.approved)
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
	repo := &fakeRepo{getRet: db.JobSubmission{ID: 5, Status: "approved"}}
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
	repo := &fakeRepo{getRet: db.JobSubmission{ID: 5, Status: "pending"}, rejectRet: db.JobSubmission{Status: "rejected"}}
	minter := &fakeMinter{}
	_, err := submission.New(repo, minter).Reject(context.Background(), 3, 5, "duplicate")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if !repo.rejectCalled {
		t.Fatal("repo.MarkRejected was not called")
	}
	if repo.rejected.ID != 5 || repo.rejected.ReviewedBy != 3 || repo.rejected.ReviewReason != "duplicate" {
		t.Errorf("reject params = %+v, want id=5 reviewer=3 reason=duplicate", repo.rejected)
	}
	if minter.called {
		t.Error("reject must not mint a job")
	}
}

func TestReject_AlreadyDecided(t *testing.T) {
	repo := &fakeRepo{getRet: db.JobSubmission{ID: 5, Status: "rejected"}}
	_, err := submission.New(repo, &fakeMinter{}).Reject(context.Background(), 3, 5, "")
	if !errors.Is(err, submission.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if repo.rejectCalled {
		t.Error("a decided submission must not be re-marked")
	}
}
