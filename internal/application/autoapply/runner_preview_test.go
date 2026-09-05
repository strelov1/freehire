package autoapply

import (
	"context"
	"errors"
	"testing"
)

type fakePreviewStore struct {
	waves [][]Claimed
	calls int

	setPreview    map[int64]ResolvedPreview
	setPreviewErr error

	parked         []int64
	parkedUnmapped map[int64][]UnmappedField
	parkErr        error

	failed  []int64
	failMax int
	failErr error
}

func (f *fakePreviewStore) ClaimForPreview(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
	f.calls++
	if f.calls > len(f.waves) {
		return nil, nil
	}
	return f.waves[f.calls-1], nil
}

func (f *fakePreviewStore) SetPreview(ctx context.Context, queueID int64, preview ResolvedPreview) error {
	if f.setPreviewErr != nil {
		return f.setPreviewErr
	}
	if f.setPreview == nil {
		f.setPreview = map[int64]ResolvedPreview{}
	}
	f.setPreview[queueID] = preview
	return nil
}

func (f *fakePreviewStore) Park(ctx context.Context, queueID int64, unmapped []UnmappedField, reason string) error {
	if f.parkErr != nil {
		return f.parkErr
	}
	f.parked = append(f.parked, queueID)
	if f.parkedUnmapped == nil {
		f.parkedUnmapped = map[int64][]UnmappedField{}
	}
	f.parkedUnmapped[queueID] = unmapped
	return nil
}

func (f *fakePreviewStore) Fail(ctx context.Context, queueID int64, errMsg string, maxAttempts int) (bool, error) {
	if f.failErr != nil {
		return false, f.failErr
	}
	f.failed = append(f.failed, queueID)
	f.failMax = maxAttempts
	return true, nil
}

type fakePreviewSidecar struct {
	result    PreviewResult
	err       error
	callCount int
}

func (f *fakePreviewSidecar) Preview(ctx context.Context, c Claimed, answers map[string]string) (PreviewResult, error) {
	f.callCount++
	return f.result, f.err
}

func TestRunPreviews_ASuccessfulPreviewIsPersisted(t *testing.T) {
	store := &fakePreviewStore{waves: [][]Claimed{{{QueueID: 1, UserID: 10, JobID: 100}}}}
	preview := ResolvedPreview{Fields: []PreviewField{{Label: "First name", Value: "Ada"}}}
	sidecar := &fakePreviewSidecar{result: PreviewResult{Preview: preview}}

	stats, err := RunPreviews(context.Background(), store, &fakeAnswers{}, sidecar, RunOptions{BatchSize: 10, Concurrency: 1})
	if err != nil {
		t.Fatalf("RunPreviews: %v", err)
	}
	if stats.Resolved != 1 {
		t.Errorf("stats = %+v, want Resolved=1", stats)
	}
	if got := store.setPreview[1]; got.Fields[0].Value != "Ada" {
		t.Errorf("SetPreview persisted %+v, want the sidecar's own preview", got)
	}
}

func TestRunPreviews_AParkedResultParksWithoutPersistingAPreview(t *testing.T) {
	store := &fakePreviewStore{waves: [][]Claimed{{{QueueID: 1}}}}
	sidecar := &fakePreviewSidecar{result: PreviewResult{Parked: true, Reason: "requires_captcha"}}

	stats, err := RunPreviews(context.Background(), store, &fakeAnswers{}, sidecar, RunOptions{BatchSize: 10, Concurrency: 1})
	if err != nil {
		t.Fatalf("RunPreviews: %v", err)
	}
	if stats.Parked != 1 {
		t.Errorf("stats = %+v, want Parked=1", stats)
	}
	if len(store.parked) != 1 || store.parked[0] != 1 {
		t.Errorf("parked = %v, want queue entry 1 parked", store.parked)
	}
	if _, ok := store.setPreview[1]; ok {
		t.Errorf("SetPreview was called for a parked attempt, want it skipped entirely")
	}
}

func TestRunPreviews_ASidecarErrorIsTreatedAsATransientFailure(t *testing.T) {
	store := &fakePreviewStore{waves: [][]Claimed{{{QueueID: 1}}}}
	sidecar := &fakePreviewSidecar{err: errors.New("launch browser: no chrome binary")}

	stats, err := RunPreviews(context.Background(), store, &fakeAnswers{}, sidecar, RunOptions{BatchSize: 10, Concurrency: 1, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("RunPreviews: %v", err)
	}
	if len(store.failed) != 1 || store.failed[0] != 1 {
		t.Errorf("failed = %v, want queue entry 1 recorded as a failure", store.failed)
	}
	_ = stats
}
