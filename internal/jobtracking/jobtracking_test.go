package jobtracking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/jobtracking"
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
	trackResult         jobtracking.Interaction
	trackErr            error
	clearProgressResult jobtracking.Interaction
	clearProgressErr    error
	untrackResult       jobtracking.Interaction
	untrackErr          error

	// recorded calls
	slugCalls  int
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
