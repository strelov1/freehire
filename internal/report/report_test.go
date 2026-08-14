package report_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/report"
)

// createArgs / resolveArgs / dismissArgs capture the primitive params the repository is
// handed, so the service tests can assert them without a db.* params struct.
type createArgs struct {
	ReportedBy      int64
	JobID           int64
	Reason          string
	Details         string
	ContactTelegram string
}

type resolveArgs struct {
	ID         int64
	ReviewedBy int64
	Note       string
}

type dismissArgs struct {
	ID           int64
	ReviewedBy   int64
	ReviewReason string
}

// fakeRepo records the params it is handed and returns canned rows, so the service tests
// run without a database (the submission_test.go precedent).
type fakeRepo struct {
	created      createArgs
	createCalled bool
	createErr    error
	createRet    report.Report

	getRet report.ReportDetail
	getErr error

	resolved      resolveArgs
	resolveCalled bool
	resolveErr    error
	resolveRet    report.Report

	dismissed     dismissArgs
	dismissCalled bool
	dismissErr    error
	dismissRet    report.Report

	filedToday int
	countErr   error
}

func (f *fakeRepo) Create(_ context.Context, reportedBy, jobID int64, reason, details, contactTelegram string) (report.Report, error) {
	f.created = createArgs{ReportedBy: reportedBy, JobID: jobID, Reason: reason, Details: details, ContactTelegram: contactTelegram}
	f.createCalled = true
	return f.createRet, f.createErr
}

func (f *fakeRepo) Get(_ context.Context, _ int64) (report.ReportDetail, error) {
	return f.getRet, f.getErr
}

func (f *fakeRepo) ListPending(_ context.Context) ([]report.ReportDetail, error) {
	return nil, nil
}

func (f *fakeRepo) MarkResolved(_ context.Context, id, reviewedBy int64, note string) (report.Report, error) {
	f.resolved, f.resolveCalled = resolveArgs{ID: id, ReviewedBy: reviewedBy, Note: note}, true
	return f.resolveRet, f.resolveErr
}

func (f *fakeRepo) MarkDismissed(_ context.Context, id, reviewedBy int64, reviewReason string) (report.Report, error) {
	f.dismissed, f.dismissCalled = dismissArgs{ID: id, ReviewedBy: reviewedBy, ReviewReason: reviewReason}, true
	return f.dismissRet, f.dismissErr
}

func (f *fakeRepo) CountFiledSince(_ context.Context, _ int64, _ time.Time) (int, error) {
	return f.filedToday, f.countErr
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
	repo := &fakeRepo{createRet: report.Report{ID: 1, Status: "pending"}}
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
	repo := &fakeRepo{createRet: report.Report{ID: 1}}
	in := report.FileInput{Reason: "spam", Details: "  not a job  ", ContactTelegram: "  @x  "}
	if _, err := report.New(repo, &fakeCloser{}).File(context.Background(), 1, 1, in); err != nil {
		t.Fatalf("File: %v", err)
	}
	if repo.created.Details != "not a job" || repo.created.ContactTelegram != "@x" {
		t.Errorf("not trimmed: details=%q contact=%q", repo.created.Details, repo.created.ContactTelegram)
	}
}

// The reason vocabulary already says what is wrong, so an explanation is
// elaboration rather than the report itself. A mandatory field mostly collects
// whatever gets past it, which is worse than an empty one: it reads as evidence.
func TestFile_AcceptsAReasonWithoutDetails(t *testing.T) {
	for _, details := range []string{"", "   "} {
		repo := &fakeRepo{}
		_, err := report.New(repo, &fakeCloser{}).File(context.Background(), 7, 1,
			report.FileInput{Reason: "spam", Details: details})
		if err != nil {
			t.Fatalf("details %q: File: %v", details, err)
		}
		if !repo.createCalled {
			t.Errorf("details %q: the report was not stored", details)
		}
		if repo.created.Details != "" {
			t.Errorf("details %q: stored as %q, want empty — blank is absent, not content",
				details, repo.created.Details)
		}
	}
}

func TestFile_ValidatesBeforePersist(t *testing.T) {
	cases := []struct {
		name string
		in   report.FileInput
	}{
		{"empty reason", report.FileInput{Reason: "", Details: "d"}},
		{"unknown reason", report.FileInput{Reason: "because", Details: "d"}},
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
		repo := &fakeRepo{createRet: report.Report{ID: 1}}
		in := report.FileInput{Reason: reason, Details: "d"}
		if _, err := report.New(repo, &fakeCloser{}).File(context.Background(), 1, 1, in); err != nil {
			t.Errorf("reason %q rejected: %v", reason, err)
		}
	}
}

// A real job seeker does not have more than a handful of postings to flag in a day; the
// cap bounds what one account can do to the moderation queue (ghostreport's precedent).
func TestFile_RefusesPastTheDailyCap(t *testing.T) {
	repo := &fakeRepo{filedToday: report.DailyCap}
	_, err := report.New(repo, &fakeCloser{}).File(context.Background(), 7, 1, validInput())
	if !errors.Is(err, report.ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
	if repo.createCalled {
		t.Error("the capped request still reached the repository")
	}
}

func TestFile_AllowsTheLastFilingUnderTheCap(t *testing.T) {
	repo := &fakeRepo{filedToday: report.DailyCap - 1, createRet: report.Report{ID: 1}}
	if _, err := report.New(repo, &fakeCloser{}).File(context.Background(), 7, 1, validInput()); err != nil {
		t.Errorf("File at cap-1: %v, want it allowed", err)
	}
	if !repo.createCalled {
		t.Error("a filing under the cap should reach the repository")
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
	repo := &fakeRepo{getRet: report.ReportDetail{Report: report.Report{ID: 5, JobID: 42, Status: "pending"}}, resolveRet: report.Report{ID: 5, Status: "resolved"}}
	closer := &fakeCloser{}
	_, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, report.ResolveInput{CloseJob: true})
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
	repo := &fakeRepo{getRet: report.ReportDetail{Report: report.Report{ID: 5, JobID: 42, Status: "pending"}}, resolveRet: report.Report{Status: "resolved"}}
	closer := &fakeCloser{}
	if _, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, report.ResolveInput{}); err != nil {
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
	repo := &fakeRepo{getRet: report.ReportDetail{Report: report.Report{ID: 5, JobID: 42, Status: "pending"}}}
	closer := &fakeCloser{err: errors.New("boom")}
	_, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, report.ResolveInput{CloseJob: true})
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
	_, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, report.ResolveInput{CloseJob: true})
	if !errors.Is(err, report.ErrReportNotFound) {
		t.Errorf("err = %v, want ErrReportNotFound", err)
	}
	if closer.called || repo.resolveCalled {
		t.Error("a missing report must not close a job or mark anything")
	}
}

func TestResolve_AlreadyDecided(t *testing.T) {
	repo := &fakeRepo{getRet: report.ReportDetail{Report: report.Report{ID: 5, Status: "resolved"}}}
	closer := &fakeCloser{}
	_, err := report.New(repo, closer).Resolve(context.Background(), 3, 5, report.ResolveInput{CloseJob: true})
	if !errors.Is(err, report.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if closer.called || repo.resolveCalled {
		t.Error("a decided report must not close a job or be re-marked")
	}
}

func TestDismiss_MarksWithReason(t *testing.T) {
	repo := &fakeRepo{getRet: report.ReportDetail{Report: report.Report{ID: 5, Status: "pending"}}, dismissRet: report.Report{Status: "dismissed"}}
	closer := &fakeCloser{}
	_, err := report.New(repo, closer).Dismiss(context.Background(), 3, 5, report.DismissInput{Reason: "not a real issue"})
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

// fakeNotifier records the decision it was handed. It also captures whether the report had
// already been marked when it ran, which is what proves the notice cannot describe an
// outcome the database never stored.
type fakeNotifier struct {
	got          report.Decision
	called       bool
	markedFirst  bool
	err          error
	observedRepo *fakeRepo
}

func (n *fakeNotifier) NotifyDecision(_ context.Context, d report.Decision) error {
	n.got, n.called = d, true
	if n.observedRepo != nil {
		n.markedFirst = n.observedRepo.resolveCalled || n.observedRepo.dismissCalled
	}
	return n.err
}

func TestResolve_NotifiesTheReporterAfterMarking(t *testing.T) {
	repo := &fakeRepo{
		getRet: report.ReportDetail{
			Report:        report.Report{ID: 5, JobID: 42, Status: "pending", Reason: "not_relevant", Details: "listed remote, source says hybrid"},
			ReporterEmail: "lina@example.test",
			JobSlug:       "senior-web-designer-incogni-1234",
			JobTitle:      "Senior Web Designer",
		},
		resolveRet: report.Report{ID: 5, Status: "resolved"},
	}
	notifier := &fakeNotifier{observedRepo: repo}
	svc := report.New(repo, &fakeCloser{}).WithNotifier(notifier)

	rev, err := svc.Resolve(context.Background(), 3, 5, report.ResolveInput{
		CloseJob: true, NotifyReporter: true, Note: "Fixed — the job is now marked hybrid",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !notifier.called {
		t.Fatal("the reporter was not notified")
	}
	if !notifier.markedFirst {
		t.Error("the notice went out before the decision was marked")
	}
	if !rev.Notified {
		t.Error("Review.Notified = false, want true after a delivered notice")
	}
	got := notifier.got
	if got.Email != "lina@example.test" || got.JobTitle != "Senior Web Designer" || got.JobSlug != "senior-web-designer-incogni-1234" {
		t.Errorf("recipient/job context not carried: %+v", got)
	}
	if got.Note != "Fixed — the job is now marked hybrid" || got.Details != "listed remote, source says hybrid" {
		t.Errorf("decision content not carried: %+v", got)
	}
	if got.Outcome != report.OutcomeResolved || !got.JobClosed {
		t.Errorf("outcome = %q closed = %v, want resolved/true", got.Outcome, got.JobClosed)
	}
}

func TestDismiss_NotifiesTheReporterWithTheReason(t *testing.T) {
	repo := &fakeRepo{
		getRet: report.ReportDetail{
			Report:        report.Report{ID: 7, JobID: 9, Status: "pending"},
			ReporterEmail: "someone@example.test",
		},
		dismissRet: report.Report{ID: 7, Status: "dismissed"},
	}
	notifier := &fakeNotifier{observedRepo: repo}
	svc := report.New(repo, &fakeCloser{}).WithNotifier(notifier)

	rev, err := svc.Dismiss(context.Background(), 3, 7, report.DismissInput{
		NotifyReporter: true, Reason: "the source really does say remote",
	})
	if err != nil {
		t.Fatalf("Dismiss: %v", err)
	}
	if !notifier.called || !notifier.markedFirst || !rev.Notified {
		t.Fatalf("dismiss notice: called=%v markedFirst=%v notified=%v", notifier.called, notifier.markedFirst, rev.Notified)
	}
	if notifier.got.Outcome != report.OutcomeDismissed || notifier.got.Note != "the source really does say remote" {
		t.Errorf("dismiss decision = %+v, want dismissed with the reason as the note", notifier.got)
	}
	if notifier.got.JobClosed {
		t.Error("dismiss must never report the job as closed")
	}
}

func TestDecide_NotifierFailureLeavesTheReportDecided(t *testing.T) {
	repo := &fakeRepo{
		getRet:     report.ReportDetail{Report: report.Report{ID: 5, Status: "pending"}},
		resolveRet: report.Report{ID: 5, Status: "resolved"},
	}
	notifier := &fakeNotifier{err: errors.New("ses is down")}
	svc := report.New(repo, &fakeCloser{}).WithNotifier(notifier)

	rev, err := svc.Resolve(context.Background(), 3, 5, report.ResolveInput{NotifyReporter: true})
	if err != nil {
		t.Fatalf("a failed notice must not fail the decision: %v", err)
	}
	if !repo.resolveCalled || rev.Report.Status != "resolved" {
		t.Error("the decision must stand when the notice fails")
	}
	if rev.Notified {
		t.Error("Review.Notified = true after a failed delivery")
	}
}

func TestDecide_DoesNotNotifyWhenNotAsked(t *testing.T) {
	repo := &fakeRepo{
		getRet:     report.ReportDetail{Report: report.Report{ID: 5, Status: "pending"}},
		resolveRet: report.Report{ID: 5, Status: "resolved"},
		dismissRet: report.Report{ID: 5, Status: "dismissed"},
	}
	notifier := &fakeNotifier{}
	svc := report.New(repo, &fakeCloser{}).WithNotifier(notifier)

	if rev, err := svc.Resolve(context.Background(), 3, 5, report.ResolveInput{}); err != nil || rev.Notified {
		t.Errorf("resolve opted out: notified=%v err=%v", rev.Notified, err)
	}
	if notifier.called {
		t.Error("the notifier ran for a decision that opted out")
	}
}

func TestDecide_WithoutANotifierIsASoftSkip(t *testing.T) {
	repo := &fakeRepo{
		getRet:     report.ReportDetail{Report: report.Report{ID: 5, Status: "pending"}},
		resolveRet: report.Report{ID: 5, Status: "resolved"},
	}
	// No WithNotifier: the SES-less wiring every dev machine runs.
	rev, err := report.New(repo, &fakeCloser{}).Resolve(context.Background(), 3, 5, report.ResolveInput{NotifyReporter: true})
	if err != nil {
		t.Fatalf("an unconfigured notifier must not fail the decision: %v", err)
	}
	if !repo.resolveCalled || rev.Notified {
		t.Errorf("marked=%v notified=%v, want marked with no notice", repo.resolveCalled, rev.Notified)
	}
}

func TestResolve_StoresTheModeratorNote(t *testing.T) {
	repo := &fakeRepo{
		getRet:     report.ReportDetail{Report: report.Report{ID: 5, JobID: 42, Status: "pending"}},
		resolveRet: report.Report{ID: 5, Status: "resolved"},
	}
	const note = "Fixed — the job is now marked hybrid"
	rev, err := report.New(repo, &fakeCloser{}).Resolve(context.Background(), 3, 5, report.ResolveInput{Note: note})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if repo.resolved.Note != note {
		t.Errorf("stored note = %q, want %q", repo.resolved.Note, note)
	}
	if rev.Report.Status != "resolved" {
		t.Errorf("returned status = %q, want resolved", rev.Report.Status)
	}
}

func TestDismiss_AlreadyDecided(t *testing.T) {
	repo := &fakeRepo{getRet: report.ReportDetail{Report: report.Report{ID: 5, Status: "dismissed"}}}
	_, err := report.New(repo, &fakeCloser{}).Dismiss(context.Background(), 3, 5, report.DismissInput{})
	if !errors.Is(err, report.ErrAlreadyDecided) {
		t.Errorf("err = %v, want ErrAlreadyDecided", err)
	}
	if repo.dismissCalled {
		t.Error("a decided report must not be re-marked")
	}
}
