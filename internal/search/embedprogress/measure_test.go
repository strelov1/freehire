package embedprogress

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeCounter struct {
	model    string
	row      db.SemanticCoverageCountsRow
	err      error
	depth    int64
	depthErr error
}

func (f *fakeCounter) SemanticOutboxClaimableDepth(ctx context.Context) (int64, error) {
	return f.depth, f.depthErr
}

func (f *fakeCounter) SemanticCoverageCounts(ctx context.Context, targetModel string) (db.SemanticCoverageCountsRow, error) {
	f.model = targetModel
	return f.row, f.err
}

func TestMeasure_CountsAgainstTheCurrentModel(t *testing.T) {
	counter := &fakeCounter{row: db.SemanticCoverageCountsRow{Eligible: 9, Covered: 4}, depth: 7}
	got, err := Measure(context.Background(), counter, func() time.Time { return t0 })
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if counter.model != search.CurrentEmbedderModel() {
		t.Errorf("counted against %q, want the current embedder model %q", counter.model, search.CurrentEmbedderModel())
	}
	if got.Eligible != 9 || got.Covered != 4 || got.QueueDepth == nil || *got.QueueDepth != 7 || !got.MeasuredAt.Equal(t0) {
		t.Errorf("snapshot = %+v, want eligible 9, covered 4, queue depth 7", got)
	}
}

func TestMeasure_PropagatesTheError(t *testing.T) {
	if _, err := Measure(context.Background(), &fakeCounter{err: errors.New("timeout")}, func() time.Time { return t0 }); err == nil {
		t.Error("a failed count produced a snapshot")
	}
}

func TestMeasure_PropagatesTheDepthError(t *testing.T) {
	counter := &fakeCounter{row: db.SemanticCoverageCountsRow{Eligible: 9, Covered: 4}, depthErr: errors.New("timeout")}
	if _, err := Measure(context.Background(), counter, func() time.Time { return t0 }); err == nil {
		t.Error("a failed depth read produced a snapshot")
	}
}
