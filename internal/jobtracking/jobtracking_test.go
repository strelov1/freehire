package jobtracking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/userjob"
)

// fakeRepo is an in-memory fake satisfying Repository.
type fakeRepo struct {
	// slug→jobID map; missing key → ErrJobNotFound
	slugs map[string]int64

	// per-method canned return (first call wins; subsequent calls return the same)
	viewResult          jobtracking.Interaction
	viewErr             error
	appliedResult       jobtracking.Interaction
	appliedErr          error
	saveResult          jobtracking.Interaction
	saveErr             error
	unsaveResult        jobtracking.Interaction
	unsaveErr           error
	dismissResult       jobtracking.Interaction
	dismissErr          error
	undismissResult     jobtracking.Interaction
	undismissErr        error
	trackResult         jobtracking.Interaction
	trackErr            error
	clearProgressResult jobtracking.Interaction
	clearProgressErr    error
	untrackResult       jobtracking.Interaction
	untrackErr          error
	listResult          []jobtracking.TrackedJob
	listErr             error
	countResult         jobtracking.Counts
	countErr            error
	viewedResult        []string
	viewedErr           error
	savedResult         []string
	savedErr            error
	dismissedResult     []string
	dismissedErr        error
	excludedResult      []int64
	excludedErr         error
	excludedLimit       int32
	pipelineResult      []userjob.StageCount
	pipelineErr         error

	// recorded calls
	slugCalls  int
	listCalls  int
	listFilter jobtracking.Filter
	trackStage *string
	trackNotes *string
}

func (f *fakeRepo) JobIDBySlug(_ context.Context, slug string) (int64, error) {
	f.slugCalls++
	id, ok := f.slugs[slug]
	if !ok {
		return 0, jobtracking.ErrJobNotFound
	}
	return id, nil
}

func (f *fakeRepo) RecordView(_ context.Context, _, _ int64) (jobtracking.Interaction, error) {
	return f.viewResult, f.viewErr
}

func (f *fakeRepo) MarkApplied(_ context.Context, _, _ int64) (jobtracking.Interaction, error) {
	return f.appliedResult, f.appliedErr
}

func (f *fakeRepo) SaveJob(_ context.Context, _, _ int64) (jobtracking.Interaction, error) {
	return f.saveResult, f.saveErr
}

func (f *fakeRepo) UnsaveJob(_ context.Context, _, _ int64) (jobtracking.Interaction, error) {
	return f.unsaveResult, f.unsaveErr
}

func (f *fakeRepo) DismissJob(_ context.Context, _, _ int64) (jobtracking.Interaction, error) {
	return f.dismissResult, f.dismissErr
}

func (f *fakeRepo) UndismissJob(_ context.Context, _, _ int64) (jobtracking.Interaction, error) {
	return f.undismissResult, f.undismissErr
}

func (f *fakeRepo) TrackJob(_ context.Context, _, _ int64, stage, notes *string) (jobtracking.Interaction, error) {
	f.trackStage = stage
	f.trackNotes = notes
	return f.trackResult, f.trackErr
}

func (f *fakeRepo) ClearJobProgress(_ context.Context, _, _ int64) (jobtracking.Interaction, error) {
	return f.clearProgressResult, f.clearProgressErr
}

func (f *fakeRepo) UntrackJob(_ context.Context, _, _ int64) (jobtracking.Interaction, error) {
	return f.untrackResult, f.untrackErr
}

func (f *fakeRepo) ListInteractions(_ context.Context, _ int64, filter jobtracking.Filter, _, _ int32) ([]jobtracking.TrackedJob, error) {
	f.listCalls++
	f.listFilter = filter
	return f.listResult, f.listErr
}

func (f *fakeRepo) CountInteractions(_ context.Context, _ int64) (jobtracking.Counts, error) {
	return f.countResult, f.countErr
}

func (f *fakeRepo) ViewedSlugs(_ context.Context, _ int64) ([]string, error) {
	return f.viewedResult, f.viewedErr
}

func (f *fakeRepo) SavedSlugs(_ context.Context, _ int64) ([]string, error) {
	return f.savedResult, f.savedErr
}

func (f *fakeRepo) DismissedSlugs(_ context.Context, _ int64) ([]string, error) {
	return f.dismissedResult, f.dismissedErr
}

func (f *fakeRepo) ExcludedJobIDs(_ context.Context, _ int64, limit int32) ([]int64, error) {
	f.excludedLimit = limit
	return f.excludedResult, f.excludedErr
}

func (f *fakeRepo) PipelineCounts(_ context.Context, _ int64) ([]userjob.StageCount, error) {
	return f.pipelineResult, f.pipelineErr
}

// helpers
func strPtr(s string) *string     { return &s }
func tPtr(t time.Time) *time.Time { return &t }

func ctx() context.Context { return context.Background() }

const (
	userID int64 = 42
	jobID  int64 = 7
	slug         = "some-job-slug"
)

func newRepo() *fakeRepo {
	return &fakeRepo{slugs: map[string]int64{slug: jobID}}
}

// ---
// 1. RecordView / MarkApplied / SaveJob — happy path and unknown slug
// ---

func TestRecordView_HappyPath(t *testing.T) {
	now := time.Now()
	repo := newRepo()
	repo.viewResult = jobtracking.Interaction{JobID: jobID, ViewedAt: tPtr(now)}
	svc := jobtracking.New(repo)

	got, err := svc.RecordView(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JobID != jobID {
		t.Errorf("JobID = %d, want %d", got.JobID, jobID)
	}
	if got.ViewedAt == nil || !got.ViewedAt.Equal(now) {
		t.Errorf("ViewedAt = %v, want %v", got.ViewedAt, now)
	}
}

func TestRecordView_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.RecordView(ctx(), userID, "no-such-slug")
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

func TestMarkApplied_HappyPath(t *testing.T) {
	now := time.Now()
	repo := newRepo()
	repo.appliedResult = jobtracking.Interaction{JobID: jobID, AppliedAt: tPtr(now)}
	svc := jobtracking.New(repo)

	got, err := svc.MarkApplied(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AppliedAt == nil {
		t.Error("AppliedAt should be set")
	}
}

func TestMarkApplied_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.MarkApplied(ctx(), userID, "missing")
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

func TestSaveJob_HappyPath(t *testing.T) {
	now := time.Now()
	repo := newRepo()
	repo.saveResult = jobtracking.Interaction{JobID: jobID, SavedAt: tPtr(now)}
	svc := jobtracking.New(repo)

	got, err := svc.SaveJob(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SavedAt == nil {
		t.Error("SavedAt should be set")
	}
}

func TestSaveJob_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.SaveJob(ctx(), userID, "missing")
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

// ---
// 2. Unsave
// ---

func TestUnsave_NoInteraction_ReturnsZero(t *testing.T) {
	repo := newRepo()
	repo.unsaveErr = jobtracking.ErrNoInteraction
	svc := jobtracking.New(repo)

	got, err := svc.Unsave(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JobID != jobID {
		t.Errorf("JobID = %d, want %d", got.JobID, jobID)
	}
	// All other fields should be nil
	if got.ViewedAt != nil || got.SavedAt != nil || got.AppliedAt != nil ||
		got.Stage != nil || got.Notes != nil {
		t.Error("zero interaction should have all nil fields except JobID")
	}
}

func TestUnsave_ExistingRow_Passthrough(t *testing.T) {
	now := time.Now()
	repo := newRepo()
	repo.unsaveResult = jobtracking.Interaction{JobID: jobID, ViewedAt: tPtr(now)}
	svc := jobtracking.New(repo)

	got, err := svc.Unsave(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ViewedAt == nil || !got.ViewedAt.Equal(now) {
		t.Errorf("ViewedAt = %v, want %v", got.ViewedAt, now)
	}
}

func TestUnsave_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.Unsave(ctx(), userID, "missing")
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

// ---
// 2b. Dismiss / Undismiss (mirrors Save / Unsave)
// ---

func TestDismiss_HappyPath(t *testing.T) {
	now := time.Now()
	repo := newRepo()
	repo.dismissResult = jobtracking.Interaction{JobID: jobID, DismissedAt: tPtr(now)}
	svc := jobtracking.New(repo)

	got, err := svc.Dismiss(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.DismissedAt == nil {
		t.Error("DismissedAt should be set")
	}
}

func TestDismiss_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.Dismiss(ctx(), userID, "missing")
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

func TestUndismiss_NoInteraction_ReturnsZero(t *testing.T) {
	repo := newRepo()
	repo.undismissErr = jobtracking.ErrNoInteraction
	svc := jobtracking.New(repo)

	got, err := svc.Undismiss(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JobID != jobID {
		t.Errorf("JobID = %d, want %d", got.JobID, jobID)
	}
	if got.ViewedAt != nil || got.SavedAt != nil || got.AppliedAt != nil ||
		got.DismissedAt != nil || got.Stage != nil || got.Notes != nil {
		t.Error("zero interaction should have all nil fields except JobID")
	}
}

func TestUndismiss_ExistingRow_Passthrough(t *testing.T) {
	now := time.Now()
	repo := newRepo()
	repo.undismissResult = jobtracking.Interaction{JobID: jobID, ViewedAt: tPtr(now)}
	svc := jobtracking.New(repo)

	got, err := svc.Undismiss(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ViewedAt == nil || !got.ViewedAt.Equal(now) {
		t.Errorf("ViewedAt = %v, want %v", got.ViewedAt, now)
	}
}

func TestUndismiss_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.Undismiss(ctx(), userID, "missing")
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

// ---
// 3. Track validation
// ---

func TestTrack_NilNil_ReturnsErrEmptyTrack(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.Track(ctx(), userID, slug, nil, nil)
	if !errors.Is(err, jobtracking.ErrEmptyTrack) {
		t.Errorf("err = %v, want ErrEmptyTrack", err)
	}
	// Validation must not touch the DB
	if repo.slugCalls != 0 {
		t.Errorf("slugCalls = %d, want 0 (validation should short-circuit before slug lookup)", repo.slugCalls)
	}
}

func TestTrack_InvalidStage_ReturnsErrInvalidStage(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.Track(ctx(), userID, slug, strPtr("bad-stage"), nil)
	if !errors.Is(err, jobtracking.ErrInvalidStage) {
		t.Errorf("err = %v, want ErrInvalidStage", err)
	}
	// Validation must not touch the DB
	if repo.slugCalls != 0 {
		t.Errorf("slugCalls = %d, want 0 (validation should short-circuit before slug lookup)", repo.slugCalls)
	}
}

func TestTrack_ValidStageOnly(t *testing.T) {
	stage := "applied"
	repo := newRepo()
	repo.trackResult = jobtracking.Interaction{JobID: jobID, Stage: strPtr(stage)}
	svc := jobtracking.New(repo)

	got, err := svc.Track(ctx(), userID, slug, strPtr(stage), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Stage == nil || *got.Stage != stage {
		t.Errorf("Stage = %v, want %q", got.Stage, stage)
	}
	// Fake should have received stage, nil notes
	if repo.trackStage == nil || *repo.trackStage != stage {
		t.Errorf("repo received stage = %v, want %q", repo.trackStage, stage)
	}
	if repo.trackNotes != nil {
		t.Errorf("repo received notes = %v, want nil", repo.trackNotes)
	}
}

func TestTrack_NotesOnly(t *testing.T) {
	notes := "great role"
	repo := newRepo()
	repo.trackResult = jobtracking.Interaction{JobID: jobID, Notes: strPtr(notes)}
	svc := jobtracking.New(repo)

	got, err := svc.Track(ctx(), userID, slug, nil, strPtr(notes))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Notes == nil || *got.Notes != notes {
		t.Errorf("Notes = %v, want %q", got.Notes, notes)
	}
	if repo.trackStage != nil {
		t.Errorf("repo received stage = %v, want nil", repo.trackStage)
	}
	if repo.trackNotes == nil || *repo.trackNotes != notes {
		t.Errorf("repo received notes = %v, want %q", repo.trackNotes, notes)
	}
}

func TestTrack_StageAndNotes(t *testing.T) {
	stage := "interview"
	notes := "second round"
	repo := newRepo()
	repo.trackResult = jobtracking.Interaction{JobID: jobID, Stage: strPtr(stage), Notes: strPtr(notes)}
	svc := jobtracking.New(repo)

	got, err := svc.Track(ctx(), userID, slug, strPtr(stage), strPtr(notes))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Stage == nil || *got.Stage != stage {
		t.Errorf("Stage = %v, want %q", got.Stage, stage)
	}
	if got.Notes == nil || *got.Notes != notes {
		t.Errorf("Notes = %v, want %q", got.Notes, notes)
	}
}

func TestTrack_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.Track(ctx(), userID, "missing", strPtr("applied"), nil)
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

// ---
// 4. ClearProgress
// ---

func TestClearProgress_HappyPath(t *testing.T) {
	now := time.Now()
	repo := newRepo()
	repo.clearProgressResult = jobtracking.Interaction{
		JobID:   jobID,
		SavedAt: tPtr(now),
		Notes:   strPtr("keep me"),
	}
	svc := jobtracking.New(repo)

	got, err := svc.ClearProgress(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JobID != jobID {
		t.Errorf("JobID = %d, want %d", got.JobID, jobID)
	}
	if got.SavedAt == nil {
		t.Error("SavedAt should be kept after ClearProgress")
	}
	if got.Notes == nil || *got.Notes != "keep me" {
		t.Errorf("Notes = %v, want %q", got.Notes, "keep me")
	}
}

func TestClearProgress_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.ClearProgress(ctx(), userID, "missing")
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
	// The clear repo method must not be called when slug resolution fails.
	if repo.clearProgressErr != nil {
		t.Error("clearProgressErr should not be set (repo clear must not be called)")
	}
}

// ---
// 5. Untrack
// ---

func TestUntrack_HappyPath(t *testing.T) {
	now := time.Now()
	repo := newRepo()
	repo.untrackResult = jobtracking.Interaction{
		JobID:    jobID,
		ViewedAt: tPtr(now),
	}
	svc := jobtracking.New(repo)

	got, err := svc.Untrack(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JobID != jobID {
		t.Errorf("JobID = %d, want %d", got.JobID, jobID)
	}
	if got.ViewedAt == nil {
		t.Error("ViewedAt should be kept after Untrack")
	}
	if got.SavedAt != nil || got.AppliedAt != nil || got.Stage != nil || got.Notes != nil {
		t.Error("board marks (saved/applied/stage/notes) should be nil after Untrack")
	}
}

func TestUntrack_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.Untrack(ctx(), userID, "missing")
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
	// The untrack repo method must not be called when slug resolution fails.
	if repo.untrackErr != nil {
		t.Error("untrackErr should not be set (repo untrack must not be called)")
	}
}

// ---
// 6. ListTracked — filter validation, default, and total selection
// ---

func TestListTracked_InvalidFilter_ShortCircuits(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.ListTracked(ctx(), userID, "bogus", 20, 0)
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

	listing, err := svc.ListTracked(ctx(), userID, "", 20, 0)
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

	listing, err := svc.ListTracked(ctx(), userID, "board", 20, 0)
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

	listing, err := svc.ListTracked(ctx(), userID, "dismissed", 20, 0)
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
	if got.Buckets.NoAnswer != 7 { // applied 5 + null-stage 2
		t.Errorf("NoAnswer = %d, want 7", got.Buckets.NoAnswer)
	}
	if got.Buckets.Interviewing != 3 {
		t.Errorf("Interviewing = %d, want 3", got.Buckets.Interviewing)
	}
}

func TestPipelinePropagatesRepoError(t *testing.T) {
	repo := &fakeRepo{pipelineErr: errors.New("boom")}
	svc := jobtracking.New(repo)
	if _, err := svc.Pipeline(context.Background(), 1); err == nil {
		t.Fatal("expected error, got nil")
	}
}
