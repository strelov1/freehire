package jobtracking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/application/userjob"
)

func TestListTracked_InvalidFilter_ShortCircuits(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.ListTracked(ctx(), userID, "bogus", jobtracking.CardFull, 20, 0)
	if !errors.Is(err, jobtracking.ErrInvalidFilter) {
		t.Errorf("err = %v, want ErrInvalidFilter", err)
	}
	// A bad filter must be rejected before any DB read.
	if repo.listCalls != 0 {
		t.Errorf("listCalls = %d, want 0 (validation should short-circuit before the listing)", repo.listCalls)
	}
}

func TestListTracked_EmptyFilterDefaultsToAll(t *testing.T) {
	repo := newRepo()
	repo.countResult = jobtracking.Counts{All: 9, Viewed: 4, Saved: 2, Applied: 3, Board: 5}
	svc := jobtracking.New(repo)

	listing, err := svc.ListTracked(ctx(), userID, "", jobtracking.CardFull, 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if listing.Filter != jobtracking.FilterAll {
		t.Errorf("Filter = %q, want %q (empty defaults to all)", listing.Filter, jobtracking.FilterAll)
	}
	if repo.listFilter != jobtracking.FilterAll {
		t.Errorf("repo received filter %q, want %q", repo.listFilter, jobtracking.FilterAll)
	}
	if listing.Total() != 9 {
		t.Errorf("Total() = %d, want 9 (the all count)", listing.Total())
	}
}

func TestListTracked_BoardFilterSelectsBoardTotal(t *testing.T) {
	repo := newRepo()
	repo.countResult = jobtracking.Counts{All: 9, Viewed: 4, Saved: 2, Applied: 3, Board: 5}
	repo.listResult = []jobtracking.TrackedJob{{Interaction: jobtracking.Interaction{JobID: jobID}}}
	svc := jobtracking.New(repo)

	listing, err := svc.ListTracked(ctx(), userID, "board", jobtracking.CardFull, 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.listFilter != jobtracking.FilterBoard {
		t.Errorf("repo received filter %q, want %q", repo.listFilter, jobtracking.FilterBoard)
	}
	if listing.Total() != 5 {
		t.Errorf("Total() = %d, want 5 (the board count, not all)", listing.Total())
	}
	if len(listing.Items) != 1 {
		t.Errorf("Items = %d, want 1 (passed through from the repo)", len(listing.Items))
	}
}

func TestViewedSlugs_Passthrough(t *testing.T) {
	repo := newRepo()
	repo.viewedResult = []string{"job-a", "job-b"}
	svc := jobtracking.New(repo)

	slugs, err := svc.ViewedSlugs(ctx(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slugs) != 2 || slugs[0] != "job-a" {
		t.Errorf("slugs = %v, want [job-a job-b]", slugs)
	}
}

func TestSavedSlugs_Passthrough(t *testing.T) {
	repo := newRepo()
	repo.savedResult = []string{"job-a", "job-b"}
	svc := jobtracking.New(repo)

	slugs, err := svc.SavedSlugs(ctx(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slugs) != 2 || slugs[0] != "job-a" {
		t.Errorf("slugs = %v, want [job-a job-b]", slugs)
	}
}

func TestDismissedSlugs_Passthrough(t *testing.T) {
	repo := newRepo()
	repo.dismissedResult = []string{"job-a", "job-b"}
	svc := jobtracking.New(repo)

	slugs, err := svc.DismissedSlugs(ctx(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slugs) != 2 || slugs[0] != "job-a" {
		t.Errorf("slugs = %v, want [job-a job-b]", slugs)
	}
}

func TestParseFilter_Dismissed(t *testing.T) {
	f, err := jobtracking.ParseFilter("dismissed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != jobtracking.FilterDismissed {
		t.Errorf("filter = %q, want %q", f, jobtracking.FilterDismissed)
	}
}

func TestListTracked_DismissedFilterSelectsDismissedTotal(t *testing.T) {
	repo := newRepo()
	repo.countResult = jobtracking.Counts{All: 9, Viewed: 4, Saved: 2, Applied: 3, Board: 5, Dismissed: 6}
	repo.listResult = []jobtracking.TrackedJob{{Interaction: jobtracking.Interaction{JobID: jobID}}}
	svc := jobtracking.New(repo)

	listing, err := svc.ListTracked(ctx(), userID, "dismissed", jobtracking.CardFull, 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.listFilter != jobtracking.FilterDismissed {
		t.Errorf("repo received filter %q, want %q", repo.listFilter, jobtracking.FilterDismissed)
	}
	if listing.Total() != 6 {
		t.Errorf("Total() = %d, want 6 (the dismissed count)", listing.Total())
	}
}

func TestExcludedJobIDs_PassthroughWithCap(t *testing.T) {
	repo := newRepo()
	repo.excludedResult = []int64{3, 1, 2}
	svc := jobtracking.New(repo)

	ids, err := svc.ExcludedJobIDs(ctx(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 || ids[0] != 3 {
		t.Errorf("ids = %v, want [3 1 2]", ids)
	}
	if repo.excludedLimit != 1000 {
		t.Errorf("repo received limit %d, want 1000 (excludedJobsCap)", repo.excludedLimit)
	}
}

func TestFeedExcludedJobIDs_PassthroughWithCap(t *testing.T) {
	repo := newRepo()
	repo.excludedResult = []int64{4, 5}
	ids, err := jobtracking.New(repo).FeedExcludedJobIDs(ctx(), userID, time.Now())
	if err != nil || len(ids) != 2 || repo.excludedLimit != 1000 {
		t.Errorf("ids=%v err=%v limit=%d, want 2 ids capped at 1000", ids, err, repo.excludedLimit)
	}
}

func TestAppliedCompanySlugs_PassthroughWithCap(t *testing.T) {
	repo := newRepo()
	repo.appliedCoResult = []string{"acme", "globex"}
	svc := jobtracking.New(repo)

	slugs, err := svc.AppliedCompanySlugs(ctx(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slugs) != 2 || slugs[0] != "acme" {
		t.Errorf("slugs = %v, want [acme globex]", slugs)
	}
	if repo.appliedCoLimit != 1000 {
		t.Errorf("repo received cap %d, want 1000 (appliedCompaniesCap)", repo.appliedCoLimit)
	}
}

func TestPipelineAggregates(t *testing.T) {
	repo := &fakeRepo{pipelineResult: []userjob.StageCount{
		{Stage: "applied", Count: 5},
		{Stage: "interview", Count: 3},
		{Stage: "", Count: 2}, // applied with no explicit stage
	}}
	svc := jobtracking.New(repo)

	got, err := svc.Pipeline(context.Background(), 1)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	if got.Applications != 10 {
		t.Errorf("Applications = %d, want 10", got.Applications)
	}
	if got.Stages["applied"] != 7 { // applied 5 + null-stage 2
		t.Errorf("Stages[applied] = %d, want 7", got.Stages["applied"])
	}
	if got.Stages["interview"] != 3 {
		t.Errorf("Stages[interview] = %d, want 3", got.Stages["interview"])
	}
}

func TestPipelinePropagatesRepoError(t *testing.T) {
	repo := &fakeRepo{pipelineErr: errors.New("boom")}
	svc := jobtracking.New(repo)
	if _, err := svc.Pipeline(context.Background(), 1); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPipelineIncludesReplyRateWhenBothSidesClearTheGate(t *testing.T) {
	repo := &fakeRepo{
		pipelineResult:  []userjob.StageCount{{Stage: "applied", Count: 1}},
		replyRateYou:    userjob.ReplyRateSide{Applications: 12, Answered: 4},
		replyRateGlobal: userjob.ReplyRateSide{Applications: 287, Answered: 97},
	}
	svc := jobtracking.New(repo)

	got, err := svc.Pipeline(context.Background(), 1)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	if got.ReplyRate == nil {
		t.Fatal("ReplyRate is nil, want a benchmark — both sides clear the sample gate")
	}
	if got.ReplyRate.You != repo.replyRateYou || got.ReplyRate.Global != repo.replyRateGlobal {
		t.Errorf("ReplyRate = %+v, want You=%+v Global=%+v", got.ReplyRate, repo.replyRateYou, repo.replyRateGlobal)
	}
}

func TestPipelineOmitsReplyRateWhenCallerBelowTheGate(t *testing.T) {
	repo := &fakeRepo{
		pipelineResult:  []userjob.StageCount{{Stage: "applied", Count: 1}},
		replyRateYou:    userjob.ReplyRateSide{Applications: 4, Answered: 1},
		replyRateGlobal: userjob.ReplyRateSide{Applications: 287, Answered: 97},
	}
	svc := jobtracking.New(repo)

	got, err := svc.Pipeline(context.Background(), 1)
	if err != nil {
		t.Fatalf("Pipeline: %v", err)
	}
	if got.ReplyRate != nil {
		t.Errorf("ReplyRate = %+v, want nil — the caller has fewer than ten observable applications", got.ReplyRate)
	}
}

// The reply-rate benchmark is a best-effort, optional signal — same convention
// internal/api/handler/company_response.go already follows ("no row, or a lookup
// error, yields nil"). A transient failure computing it must not take down the whole
// pipeline endpoint, which existed and was reliable before this benchmark did.
func TestPipelineIsUnaffectedByReplyRateRepoError(t *testing.T) {
	repo := &fakeRepo{
		pipelineResult: []userjob.StageCount{{Stage: "applied", Count: 1}},
		replyRateErr:   errors.New("boom"),
	}
	svc := jobtracking.New(repo)

	got, err := svc.Pipeline(context.Background(), 1)
	if err != nil {
		t.Fatalf("Pipeline: %v, want no error — a reply-rate lookup failure must degrade to absent, not fail the endpoint", err)
	}
	if got.Applications != 1 {
		t.Errorf("Applications = %d, want 1 — the stage counts must still come through", got.Applications)
	}
	if got.ReplyRate != nil {
		t.Errorf("ReplyRate = %+v, want nil after a lookup error", got.ReplyRate)
	}
}
