package jobtracking

import (
	"context"
	"testing"
)

type fakeEmployerStageRepo struct {
	stage      string
	byEmployer bool
	found      bool
	// moved is what SetIfUnchanged answers; set records the call.
	moved bool
	set   *[2]string // expected, stage
}

func (f *fakeEmployerStageRepo) CandidateStage(context.Context, int64, int64) (string, bool, bool, error) {
	return f.stage, f.byEmployer, f.found, nil
}

func (f *fakeEmployerStageRepo) SetIfUnchanged(_ context.Context, _, _ int64, expected, stage string) (bool, error) {
	f.set = &[2]string{expected, stage}
	return f.moved, nil
}

func TestEmployerStagesMirror(t *testing.T) {
	t.Run("sets an allowed move against the stage it read", func(t *testing.T) {
		repo := &fakeEmployerStageRepo{stage: "applied", found: true, moved: true}
		moved, err := (&EmployerStages{repo: repo}).Mirror(context.Background(), 1, 2, "interview")
		if err != nil || !moved {
			t.Fatalf("Mirror = %v, %v; want moved", moved, err)
		}
		if repo.set == nil || *repo.set != [2]string{"applied", "interview"} {
			t.Fatalf("SetIfUnchanged got %v, want expected=applied stage=interview", repo.set)
		}
	})
	t.Run("reports a lost compare-and-set as not moved", func(t *testing.T) {
		repo := &fakeEmployerStageRepo{stage: "applied", found: true, moved: false}
		moved, err := (&EmployerStages{repo: repo}).Mirror(context.Background(), 1, 2, "interview")
		if err != nil || moved {
			t.Fatalf("Mirror = %v, %v; want not moved", moved, err)
		}
	})
	t.Run("leaves a candidate without a record without one", func(t *testing.T) {
		repo := &fakeEmployerStageRepo{found: false}
		moved, err := (&EmployerStages{repo: repo}).Mirror(context.Background(), 1, 2, "rejected")
		if err != nil || moved || repo.set != nil {
			t.Fatalf("Mirror = %v, %v, set %v; want nothing written", moved, err, repo.set)
		}
	})
	t.Run("never reopens what the candidate settled", func(t *testing.T) {
		repo := &fakeEmployerStageRepo{stage: "withdrawn", found: true, moved: true}
		moved, err := (&EmployerStages{repo: repo}).Mirror(context.Background(), 1, 2, "interview")
		if err != nil || moved || repo.set != nil {
			t.Fatalf("Mirror = %v, %v, set %v; want nothing written", moved, err, repo.set)
		}
	})
}
