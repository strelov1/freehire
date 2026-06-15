package report_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/report"
)

// fakeRepo records the params it is handed and returns canned rows, so the service tests
// run without a database (the submission_test.go precedent).
type fakeRepo struct {
	created      db.CreateReportParams
	createCalled bool
	createErr    error
	createRet    db.JobReport

	getRet db.JobReport
	getErr error

	resolved      db.MarkReportResolvedParams
	resolveCalled bool
	resolveErr    error
	resolveRet    db.JobReport

	dismissed     db.MarkReportDismissedParams
	dismissCalled bool
	dismissErr    error
	dismissRet    db.JobReport
}

func (f *fakeRepo) Create(_ context.Context, p db.CreateReportParams) (db.JobReport, error) {
	f.created, f.createCalled = p, true
	return f.createRet, f.createErr
}

func (f *fakeRepo) Get(_ context.Context, _ int64) (db.JobReport, error) {
	return f.getRet, f.getErr
}

func (f *fakeRepo) ListPending(_ context.Context) ([]db.ListPendingReportsRow, error) {
	return nil, nil
}

func (f *fakeRepo) MarkResolved(_ context.Context, p db.MarkReportResolvedParams) (db.JobReport, error) {
	f.resolved, f.resolveCalled = p, true
	return f.resolveRet, f.resolveErr
}

func (f *fakeRepo) MarkDismissed(_ context.Context, p db.MarkReportDismissedParams) (db.JobReport, error) {
	f.dismissed, f.dismissCalled = p, true
	return f.dismissRet, f.dismissErr
}

// fakeCloser stands in for the job-lifecycle soft-close: it records the close call.
type fakeCloser struct {
	jobID  int64
	called bool
	err    error
}

func (c *fakeCloser) Close(_ context.Context, jobID int64) error {
	c.jobID, c.called = jobID, true
	return c.err
}

func validInput() report.FileInput {
	return report.FileInput{Reason: "fraud", Details: "asks for payment", ContactTelegram: "@me"}
}

func TestFile_PersistsPendingWithOwnerAndJob(t *testing.T) {
	repo := &fakeRepo{createRet: db.JobReport{ID: 1, Status: "pending"}}
	svc := report.New(repo, &fakeCloser{})

	_, err := svc.File(context.Background(), 7, 42, validInput())
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("repo.Create was not called")
	}
	got := repo.created
	if got.ReportedBy != 7 || got.JobID != 42 {
		t.Errorf("ownership not carried: reportedBy=%d jobID=%d, want 7/42", got.ReportedBy, got.JobID)
	}
	if got.Reason != "fraud" || got.Details != "asks for payment" || got.ContactTelegram != "@me" {
		t.Errorf("content not carried through: %+v", got)
	}
}

func TestFile_TrimsDetailsAndContact(t *testing.T) {
	repo := &fakeRepo{createRet: db.JobReport{ID: 1}}
	in := report.FileInput{Reason: "spam", Details: "  not a job  ", ContactTelegram: "  @x  "}
	if _, err := report.New(repo, &fakeCloser{}).File(context.Background(), 1, 1, in); err != nil {
		t.Fatalf("File: %v", err)
	}
	if repo.created.Details != "not a job" || repo.created.ContactTelegram != "@x" {
		t.Errorf("not trimmed: details=%q contact=%q", repo.created.Details, repo.created.ContactTelegram)
	}
}

func TestFile_ValidatesBeforePersist(t *testing.T) {
	cases := []struct {
		name string
		in   report.FileInput
	}{
		{"empty reason", report.FileInput{Reason: "", Details: "d"}},
		{"unknown reason", report.FileInput{Reason: "because", Details: "d"}},
		{"missing details", report.FileInput{Reason: "spam", Details: ""}},
		{"blank details", report.FileInput{Reason: "spam", Details: "   "}},
		{"details too long", report.FileInput{Reason: "spam", Details: strings.Repeat("x", 5001)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := report.New(repo, &fakeCloser{}).File(context.Background(), 7, 1, tc.in)
			if !errors.Is(err, report.ErrInvalid) {
				t.Errorf("err = %v, want report.ErrInvalid", err)
			}
			if repo.createCalled {
				t.Error("repo.Create should not be called on invalid input")
			}
		})
	}
}

func TestFile_AcceptsEveryReason(t *testing.T) {
	for _, reason := range []string{"no_response", "not_relevant", "spam", "fraud", "other"} {
		repo := &fakeRepo{createRet: db.JobReport{ID: 1}}
		in := report.FileInput{Reason: reason, Details: "d"}
		if _, err := report.New(repo, &fakeCloser{}).File(context.Background(), 1, 1, in); err != nil {
			t.Errorf("reason %q rejected: %v", reason, err)
		}
	}
}

func TestFile_PropagatesDuplicateOpen(t *testing.T) {
	repo := &fakeRepo{createErr: report.ErrDuplicateOpen}
	_, err := report.New(repo, &fakeCloser{}).File(context.Background(), 7, 1, validInput())
	if !errors.Is(err, report.ErrDuplicateOpen) {
		t.Errorf("err = %v, want ErrDuplicateOpen", err)
	}
}

func TestResolve_ClosesJobWhenAsked(t *testing.T) {
	repo := &fakeRepo{getRet: db.JobReport{ID: 5, JobID: 42, Status: "pending"}, resolveRet: db.JobReport{ID: 5, Status: "resolved"}}
	closer := &fakeCloser{}
	_, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, true)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !closer.called || closer.jobID != 42 {
		t.Errorf("closer not called with the reported job id: called=%v jobID=%d", closer.called, closer.jobID)
	}
	if !repo.resolveCalled || repo.resolved.ID != 5 || repo.resolved.ReviewedBy != 3 {
		t.Errorf("resolve params = %+v, want id=5 reviewer=3", repo.resolved)
	}
}

func TestResolve_LeavesJobOpenWhenNotAsked(t *testing.T) {
	repo := &fakeRepo{getRet: db.JobReport{ID: 5, JobID: 42, Status: "pending"}, resolveRet: db.JobReport{Status: "resolved"}}
	closer := &fakeCloser{}
	if _, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, false); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if closer.called {
		t.Error("the job must not be closed when close_job is false")
	}
	if !repo.resolveCalled {
		t.Error("repo.MarkResolved was not called")
	}
}

func TestResolve_CloseErrorAbortsBeforeMark(t *testing.T) {
	repo := &fakeRepo{getRet: db.JobReport{ID: 5, JobID: 42, Status: "pending"}}
	closer := &fakeCloser{err: errors.New("boom")}
	_, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, true)
	if err == nil {
		t.Fatal("expected the close error to propagate")
	}
	if repo.resolveCalled {
		t.Error("the report must not be marked resolved when the close failed")
	}
}

func TestResolve_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: report.ErrReportNotFound}
	closer := &fakeCloser{}
	_, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, true)
	if !errors.Is(err, report.ErrReportNotFound) {
		t.Errorf("err = %v, want ErrReportNotFound", err)
	}
	if closer.called || repo.resolveCalled {
		t.Error("a missing report must not close a job or mark anything")
	}
}

func TestResolve_AlreadyDecided(t *testing.T) {
	repo := &fakeRepo{getRet: db.JobReport{ID: 5, Status: "resolved"}}
	closer := &fakeCloser{}
	_, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, true)
	if !errors.Is(err, report.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if closer.called || repo.resolveCalled {
		t.Error("a decided report must not close a job or be re-marked")
	}
}

func TestDismiss_MarksWithReason(t *testing.T) {
	repo := &fakeRepo{getRet: db.JobReport{ID: 5, Status: "pending"}, dismissRet: db.JobReport{Status: "dismissed"}}
	closer := &fakeCloser{}
	_, err := report.New(repo, closer).Dismiss(context.Background(), 3, 5, "not a real issue")
	if err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if !repo.dismissCalled || repo.dismissed.ID != 5 || repo.dismissed.ReviewedBy != 3 || repo.dismissed.ReviewReason != "not a real issue" {
		t.Errorf("dismiss params = %+v, want id=5 reviewer=3 reason set", repo.dismissed)
	}
	if closer.called {
		t.Error("dismiss must not close the job")
	}
}

func TestDismiss_AlreadyDecided(t *testing.T) {
	repo := &fakeRepo{getRet: db.JobReport{ID: 5, Status: "dismissed"}}
	_, err := report.New(repo, &fakeCloser{}).Dismiss(context.Background(), 3, 5, "")
	if !errors.Is(err, report.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if repo.dismissCalled {
		t.Error("a decided report must not be re-marked")
	}
}
