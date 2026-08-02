package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/appevent"
	"github.com/strelov1/freehire/internal/jobtracking"
	"github.com/strelov1/freehire/internal/userjob"
)

// trackingRepo is an in-memory jobtracking.Repository: one known slug, and a
// record of the calls the tools made through the service.
type trackingRepo struct {
	slug  string
	jobID int64

	saved   bool
	unsaved bool
	applied bool
	// appliedSource is the ledger provenance the tool declared for the recording.
	appliedSource string
	gotStage      *string
	gotNotes      *string
	listedFor     jobtracking.Filter
}

func (r *trackingRepo) JobIDBySlug(_ context.Context, slug string) (int64, error) {
	if slug != r.slug {
		return 0, jobtracking.ErrJobNotFound
	}
	return r.jobID, nil
}

func (r *trackingRepo) RecordView(_ context.Context, _, jobID int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: jobID}, nil
}

func (r *trackingRepo) MarkApplied(_ context.Context, _, jobID int64, source string) (jobtracking.Interaction, error) {
	r.applied = true
	r.appliedSource = source
	now := time.Now()
	return jobtracking.Interaction{JobID: jobID, AppliedAt: &now}, nil
}

func (r *trackingRepo) MarkAppliedAt(_ context.Context, _, jobID int64, at time.Time, source string) (jobtracking.Interaction, error) {
	r.applied = true
	r.appliedSource = source
	return jobtracking.Interaction{JobID: jobID, AppliedAt: &at}, nil
}

func (r *trackingRepo) MarkAppliedOn(_ context.Context, _, jobID int64, at time.Time, source string) (jobtracking.Interaction, error) {
	r.applied = true
	r.appliedSource = source
	return jobtracking.Interaction{JobID: jobID, AppliedAt: &at}, nil
}

func (r *trackingRepo) SaveJob(_ context.Context, _, jobID int64) (jobtracking.Interaction, error) {
	r.saved = true
	now := time.Now()
	return jobtracking.Interaction{JobID: jobID, SavedAt: &now}, nil
}

func (r *trackingRepo) UnsaveJob(_ context.Context, _, jobID int64) (jobtracking.Interaction, error) {
	r.unsaved = true
	return jobtracking.Interaction{JobID: jobID}, nil
}

func (r *trackingRepo) DismissJob(_ context.Context, _, jobID int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: jobID}, nil
}

func (r *trackingRepo) UndismissJob(_ context.Context, _, jobID int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: jobID}, nil
}

func (r *trackingRepo) TrackJob(_ context.Context, _, jobID int64, stage, notes *string, _ string) (jobtracking.Interaction, error) {
	r.gotStage, r.gotNotes = stage, notes
	return jobtracking.Interaction{JobID: jobID, Stage: stage, Notes: notes}, nil
}

func (r *trackingRepo) ClearJobProgress(_ context.Context, _, jobID int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: jobID}, nil
}

func (r *trackingRepo) UntrackJob(_ context.Context, _, jobID int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{JobID: jobID}, nil
}

// The application-addressed writes are the board's, not the assistant's; these exist
// to satisfy the interface.
func (r *trackingRepo) TrackApplication(context.Context, int64, int64, *string, *string, string) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{}, nil
}

func (r *trackingRepo) ClearApplicationProgress(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{}, nil
}

func (r *trackingRepo) UntrackApplication(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{}, nil
}

func (r *trackingRepo) ListInteractions(_ context.Context, _ int64, filter jobtracking.Filter, _, _ int32) ([]jobtracking.TrackedJob, error) {
	r.listedFor = filter
	return nil, nil
}

func (r *trackingRepo) CountInteractions(context.Context, int64) (jobtracking.Counts, error) {
	return jobtracking.Counts{}, nil
}

func (r *trackingRepo) PipelineCounts(context.Context, int64) ([]userjob.StageCount, error) {
	return nil, nil
}

func (r *trackingRepo) ViewedSlugs(context.Context, int64) ([]string, error)    { return nil, nil }
func (r *trackingRepo) SavedSlugs(context.Context, int64) ([]string, error)     { return nil, nil }
func (r *trackingRepo) DismissedSlugs(context.Context, int64) ([]string, error) { return nil, nil }

func (r *trackingRepo) ExcludedJobIDs(context.Context, int64, int32) ([]int64, error) {
	return nil, nil
}

func trackingAPI(repo *trackingRepo) *assistantHandlers {
	return &assistantHandlers{tracking: &trackingHandlers{tracking: jobtracking.New(repo)}}
}

func TestSaveJobToolSavesForTheCaller(t *testing.T) {
	repo := &trackingRepo{slug: "go-dev-acme", jobID: 11}
	a := trackingAPI(repo)

	tool := toolByName(t, a.assistantTrackingTools(), "save_job")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(`{"slug":"go-dev-acme"}`)); err != nil {
		t.Fatalf("save_job: %v", err)
	}
	if !repo.saved {
		t.Error("save_job did not reach the tracking service")
	}
}

func TestApplyJobToolMarksApplied(t *testing.T) {
	repo := &trackingRepo{slug: "go-dev-acme", jobID: 11}
	a := trackingAPI(repo)

	tool := toolByName(t, a.assistantTrackingTools(), "apply_job")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(`{"slug":"go-dev-acme"}`)); err != nil {
		t.Fatalf("apply_job: %v", err)
	}
	if !repo.applied {
		t.Error("apply_job did not mark the vacancy applied")
	}
	// The tool acts as the session owner, but the ledger records who observed the
	// application. Crediting the agent's recording to the person would put a
	// machine-recorded row next to hand-recorded ones with nothing to tell them apart.
	if repo.appliedSource != appevent.SourceAssistant {
		t.Errorf("apply_job recorded provenance %q, want %q", repo.appliedSource, appevent.SourceAssistant)
	}
}

func TestUnsaveJobToolClearsTheBookmark(t *testing.T) {
	repo := &trackingRepo{slug: "go-dev-acme", jobID: 11}
	a := trackingAPI(repo)

	tool := toolByName(t, a.assistantTrackingTools(), "unsave_job")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(`{"slug":"go-dev-acme"}`)); err != nil {
		t.Fatalf("unsave_job: %v", err)
	}
	if !repo.unsaved {
		t.Error("unsave_job did not clear the bookmark")
	}
}

func TestTrackJobToolSetsStageAndNote(t *testing.T) {
	repo := &trackingRepo{slug: "go-dev-acme", jobID: 11}
	a := trackingAPI(repo)

	tool := toolByName(t, a.assistantTrackingTools(), "track_job")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"slug":"go-dev-acme","stage":"interview","note":"call on Tuesday"}`))
	if err != nil {
		t.Fatalf("track_job: %v", err)
	}
	if repo.gotStage == nil || *repo.gotStage != "interview" {
		t.Errorf("stage = %v, want interview", repo.gotStage)
	}
	if repo.gotNotes == nil || *repo.gotNotes != "call on Tuesday" {
		t.Errorf("note = %v, want the supplied text", repo.gotNotes)
	}
}

func TestTrackJobToolRejectsAnInventedStage(t *testing.T) {
	repo := &trackingRepo{slug: "go-dev-acme", jobID: 11}
	a := trackingAPI(repo)

	tool := toolByName(t, a.assistantTrackingTools(), "track_job")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(`{"slug":"go-dev-acme","stage":"vibing"}`))
	if err == nil {
		t.Fatal("an invented stage must be rejected, so the board's vocabulary stays closed")
	}
	if !strings.Contains(err.Error(), "interview") {
		t.Errorf("error = %v, want it to list the valid stages so the model can correct itself", err)
	}
}

func TestTrackingToolOnAnUnknownSlugFails(t *testing.T) {
	repo := &trackingRepo{slug: "go-dev-acme", jobID: 11}
	a := trackingAPI(repo)

	tool := toolByName(t, a.assistantTrackingTools(), "save_job")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(`{"slug":"does-not-exist"}`)); err == nil {
		t.Fatal("want an error naming the unknown vacancy")
	}
}

func TestMyJobsToolPassesTheFilter(t *testing.T) {
	repo := &trackingRepo{slug: "go-dev-acme", jobID: 11}
	a := trackingAPI(repo)

	tool := toolByName(t, a.assistantTrackingTools(), "my_jobs")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(`{"filter":"applied"}`)); err != nil {
		t.Fatalf("my_jobs: %v", err)
	}
	if repo.listedFor != jobtracking.FilterApplied {
		t.Errorf("listed filter = %q, want applied", repo.listedFor)
	}
}
