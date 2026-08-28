package jobtracking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/application/userjob"
	"github.com/strelov1/freehire/internal/job/applydate"
)

// fakeRepo is an in-memory fake satisfying Repository.
type fakeRepo struct {
	// slug→jobID map; missing key → ErrJobNotFound
	slugs map[string]int64

	// per-method canned return (first call wins; subsequent calls return the same)
	viewResult    jobtracking.Interaction
	viewErr       error
	appliedResult jobtracking.Interaction
	appliedErr    error
	// appliedAt captures the date handed to MarkAppliedAt.
	appliedAt time.Time
	// appliedSource captures the ledger provenance the caller declared.
	appliedSource string
	// redatedAt captures the day handed to MarkAppliedOn, so a test can tell the stated-date
	// path from the mail-derived one.
	redatedAt           time.Time
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
	// The application-addressed writes record the id they were handed, so a test can
	// pin that the service reached the application directly rather than by a slug.
	appIDCalls []int64
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

func (f *fakeRepo) MarkApplied(_ context.Context, _, _ int64, source string) (jobtracking.Interaction, error) {
	f.appliedSource = source
	return f.appliedResult, f.appliedErr
}

// MarkAppliedAt records the date it was handed, so a test can assert the service
// passed the mail's timestamp through rather than substituting its own.
func (f *fakeRepo) MarkAppliedAt(_ context.Context, _, _ int64, at time.Time, source string) (jobtracking.Interaction, error) {
	f.appliedAt = at
	f.appliedSource = source
	return f.appliedResult, f.appliedErr
}

// MarkAppliedOn records the stated day. It is a distinct method from MarkAppliedAt because the
// two differ in whether they may overwrite a date already stored, and a test that could not tell
// them apart would not notice them being wired together.
func (f *fakeRepo) MarkAppliedOn(_ context.Context, _, _ int64, at time.Time, source string) (jobtracking.Interaction, error) {
	f.redatedAt = at
	f.appliedSource = source
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

func (f *fakeRepo) TrackJob(_ context.Context, _, _ int64, stage, notes *string, _ string) (jobtracking.Interaction, error) {
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

func (f *fakeRepo) TrackApplication(_ context.Context, _, appID int64, stage, notes *string, _ string) (jobtracking.Interaction, error) {
	f.appIDCalls = append(f.appIDCalls, appID)
	f.trackStage = stage
	f.trackNotes = notes
	return f.trackResult, f.trackErr
}

func (f *fakeRepo) ClearApplicationProgress(_ context.Context, _, appID int64) (jobtracking.Interaction, error) {
	f.appIDCalls = append(f.appIDCalls, appID)
	return f.clearProgressResult, f.clearProgressErr
}

func (f *fakeRepo) UntrackApplication(_ context.Context, _, appID int64) (jobtracking.Interaction, error) {
	f.appIDCalls = append(f.appIDCalls, appID)
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

	got, err := svc.MarkApplied(ctx(), userID, slug, appevent.SourceUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AppliedAt == nil {
		t.Error("AppliedAt should be set")
	}
}

// The dated path hands the repository the caller's timestamp untouched — the
// service must not substitute its own clock, or an application reconstructed
// from old mail would read as applied today.
func TestMarkAppliedAt_PassesTheDateThrough(t *testing.T) {
	when := time.Now().Add(-21 * 24 * time.Hour)
	repo := newRepo()
	repo.appliedResult = jobtracking.Interaction{JobID: jobID, AppliedAt: tPtr(when)}
	svc := jobtracking.New(repo)

	if _, err := svc.MarkAppliedAt(ctx(), userID, slug, when, appevent.SourceMailGmail); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.appliedAt.Equal(when) {
		t.Errorf("repository received %v, want the supplied %v", repo.appliedAt, when)
	}
}

func TestMarkAppliedAt_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.MarkAppliedAt(ctx(), userID, "missing", time.Now(), appevent.SourceMailGmail)
	if !errors.Is(err, jobtracking.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

func TestMarkApplied_UnknownSlug(t *testing.T) {
	repo := newRepo()
	svc := jobtracking.New(repo)

	_, err := svc.MarkApplied(ctx(), userID, "missing", appevent.SourceUser)
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

	_, err := svc.Track(ctx(), userID, slug, nil, nil, appevent.SourceUser)

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

	_, err := svc.Track(ctx(), userID, slug, strPtr("bad-stage"), nil, appevent.SourceUser)
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

	got, err := svc.Track(ctx(), userID, slug, strPtr(stage), nil, appevent.SourceUser)
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

	got, err := svc.Track(ctx(), userID, slug, nil, strPtr(notes), appevent.SourceUser)
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

	got, err := svc.Track(ctx(), userID, slug, strPtr(stage), strPtr(notes), appevent.SourceUser)
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

	_, err := svc.Track(ctx(), userID, "missing", strPtr("applied"), nil, appevent.SourceUser)
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

// The candidate stating the day beats anything we inferred, so this path overwrites a date
// already recorded — the opposite of MarkAppliedAt, whose date is read off employer mail and is
// only an upper bound. The two therefore reach different repository methods.
func TestMarkAppliedOn_OverwritesADateAlreadyRecorded(t *testing.T) {
	sent := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	repo := newRepo()
	repo.appliedResult = jobtracking.Interaction{JobID: jobID, AppliedAt: tPtr(sent)}
	svc := jobtracking.New(repo)

	if _, err := svc.MarkAppliedOn(ctx(), userID, slug, sent, now, appevent.SourceUser); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.redatedAt.Equal(sent) {
		t.Errorf("repository re-dated to %v, want the stated %v", repo.redatedAt, sent)
	}
}

// The window is checked here rather than at the HTTP door, so the CLI and the in-app assistant —
// which call this service directly and never pass through Fiber — are bounded by the same rule.
func TestMarkAppliedOn_RefusesADateOutsideTheWindow(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"tomorrow":          now.AddDate(0, 0, 1),
		"older than a year": now.AddDate(0, 0, -400),
	}
	for name, day := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newRepo()
			svc := jobtracking.New(repo)

			_, err := svc.MarkAppliedOn(ctx(), userID, slug, day, now, appevent.SourceUser)
			if !errors.Is(err, applydate.ErrOutOfRange) {
				t.Errorf("err = %v, want ErrAppliedOnOutOfRange", err)
			}
			if !repo.redatedAt.IsZero() {
				t.Error("an unbelievable date reached the repository")
			}
		})
	}
}

// Stating today must work at every hour of the day. The day is stored at noon UTC, so validating
// that derived instant rather than the day itself refuses "today" for the whole UTC morning —
// and, for a caller east of UTC+12, refuses their today permanently. The window bounds the day a
// person names; the hour the row happens to be stored at is not part of it.
func TestMarkAppliedOn_AcceptsTodayAtAnyHour(t *testing.T) {
	day := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	for _, hour := range []int{0, 6, 11, 12, 23} {
		now := time.Date(2026, 8, 2, hour, 0, 0, 0, time.UTC)
		repo := newRepo()
		repo.appliedResult = jobtracking.Interaction{JobID: jobID, AppliedAt: tPtr(day)}
		svc := jobtracking.New(repo)

		if _, err := svc.MarkAppliedOn(ctx(), userID, slug, day, now, appevent.SourceUser); err != nil {
			t.Errorf("now=%02d:00 UTC: %v", hour, err)
		}
	}
}

// The day reaches storage as noon UTC. Midnight would render as the previous date for every
// reader west of Greenwich, so the card would show a day earlier than the one they typed.
func TestMarkAppliedOn_StoresTheDayAtNoon(t *testing.T) {
	day := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	repo := newRepo()
	svc := jobtracking.New(repo)

	if _, err := svc.MarkAppliedOn(ctx(), userID, slug, day, now, appevent.SourceUser); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	if !repo.redatedAt.Equal(want) {
		t.Errorf("repository received %v, want %v", repo.redatedAt, want)
	}
}

// fakeReminders records the reminder side effects the service asks for. It is a
// two-method port, so a handwritten fake stays this small — which is the point of
// declaring the interface here rather than reaching for engage's own service.
type fakeReminders struct {
	scheduled []int64
	cancelled []int64
	err       error
}

func (f *fakeReminders) ScheduleOnSave(_ context.Context, _, jobID int64) error {
	f.scheduled = append(f.scheduled, jobID)
	return f.err
}

func (f *fakeReminders) Cancel(_ context.Context, _, jobID int64) error {
	f.cancelled = append(f.cancelled, jobID)
	return f.err
}

// TestSaveJob_SchedulesTheReminder is the regression this side effect was moved
// down for: it used to live in the Fiber handler, so a job saved through the in-app
// assistant — which calls this service directly and issues no HTTP request — never
// got a reminder row. The test drives the service, exactly as that caller does.
func TestSaveJob_SchedulesTheReminder(t *testing.T) {
	repo := newRepo()
	repo.saveResult = jobtracking.Interaction{JobID: jobID, SavedAt: tPtr(time.Now())}
	rem := &fakeReminders{}
	svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

	if _, err := svc.SaveJob(ctx(), userID, slug); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rem.scheduled) != 1 || rem.scheduled[0] != jobID {
		t.Errorf("scheduled = %v, want exactly [%d]", rem.scheduled, jobID)
	}
	if len(rem.cancelled) != 0 {
		t.Errorf("cancelled = %v, want none", rem.cancelled)
	}
}

// TestEndingTheIntentCancelsTheReminder covers every way an application stops being
// pending. MarkAppliedAt is the mail-reconstruction path, which never cancelled at
// all while the rule lived at the HTTP door.
func TestEndingTheIntentCancelsTheReminder(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(*jobtracking.Service) error
	}{
		{"apply", func(s *jobtracking.Service) error {
			_, err := s.MarkApplied(ctx(), userID, slug, appevent.SourceAssistant)
			return err
		}},
		{"apply from mail", func(s *jobtracking.Service) error {
			_, err := s.MarkAppliedAt(ctx(), userID, slug, time.Now().Add(-time.Hour), appevent.SourceMailGmail)
			return err
		}},
		{"apply on a stated day", func(s *jobtracking.Service) error {
			day := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
			now := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
			_, err := s.MarkAppliedOn(ctx(), userID, slug, day, now, appevent.SourceUser)
			return err
		}},
		{"unsave", func(s *jobtracking.Service) error {
			_, err := s.Unsave(ctx(), userID, slug)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepo()
			rem := &fakeReminders{}
			svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

			if err := tc.act(svc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(rem.cancelled) != 1 || rem.cancelled[0] != jobID {
				t.Errorf("cancelled = %v, want exactly [%d]", rem.cancelled, jobID)
			}
			if len(rem.scheduled) != 0 {
				t.Errorf("scheduled = %v, want none", rem.scheduled)
			}
		})
	}
}

// TestUnsave_CancelsEvenWithNothingToClear pins the idempotent case: unsaving a job
// that carries no interaction row still withdraws the intent, because the caller
// asked for it and the reminder may exist regardless.
func TestUnsave_CancelsEvenWithNothingToClear(t *testing.T) {
	repo := newRepo()
	repo.unsaveErr = jobtracking.ErrNoInteraction
	rem := &fakeReminders{}
	svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

	got, err := svc.Unsave(ctx(), userID, slug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JobID != jobID {
		t.Errorf("JobID = %d, want %d", got.JobID, jobID)
	}
	if len(rem.cancelled) != 1 {
		t.Errorf("cancelled = %v, want exactly one", rem.cancelled)
	}
}

// TestReminderIsBestEffort pins both halves of "best effort": a failing reminder
// never fails the tracking write, and a service built without the port does the
// write and nothing else.
func TestReminderIsBestEffort(t *testing.T) {
	t.Run("a reminder failure does not fail the save", func(t *testing.T) {
		repo := newRepo()
		repo.saveResult = jobtracking.Interaction{JobID: jobID, SavedAt: tPtr(time.Now())}
		rem := &fakeReminders{err: errors.New("reminder store is down")}
		svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

		got, err := svc.SaveJob(ctx(), userID, slug)
		if err != nil {
			t.Fatalf("save should succeed despite the reminder: %v", err)
		}
		if got.SavedAt == nil {
			t.Error("SavedAt should be set")
		}
	})

	t.Run("no port configured is silent", func(t *testing.T) {
		repo := newRepo()
		repo.saveResult = jobtracking.Interaction{JobID: jobID, SavedAt: tPtr(time.Now())}
		svc := jobtracking.New(repo)

		if _, err := svc.SaveJob(ctx(), userID, slug); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestFailedWriteSchedulesNothing keeps the side effect downstream of the write it
// belongs to: a save that did not happen must not leave a reminder behind.
func TestFailedWriteSchedulesNothing(t *testing.T) {
	repo := newRepo()
	repo.saveErr = errors.New("write failed")
	rem := &fakeReminders{}
	svc := jobtracking.New(repo, jobtracking.WithReminders(rem))

	if _, err := svc.SaveJob(ctx(), userID, slug); err == nil {
		t.Fatal("expected the save to fail")
	}
	if len(rem.scheduled) != 0 {
		t.Errorf("scheduled = %v, want none", rem.scheduled)
	}
}
