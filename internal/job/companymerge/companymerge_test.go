package companymerge

import (
	"context"
	"errors"
	"testing"
)

// MoveJobs drains until a chunk moves nothing, so a short last chunk still gets one more
// look: rows written onto the alias mid-run are not left behind.
func TestMoveJobsDrainsUntilAChunkMovesNothing(t *testing.T) {
	left := 1200
	calls := 0
	rekey := func(_ context.Context, alias, canonical string, chunk int) (int64, error) {
		calls++
		n := min(left, chunk)
		left -= n
		return int64(n), nil
	}
	moved, err := MoveJobs(context.Background(), rekey, "a", "b", 500)
	if err != nil || moved != 1200 || left != 0 {
		t.Fatalf("moved %d, left %d, err %v; want 1200, 0, nil", moved, left, err)
	}
	if calls != 4 {
		t.Errorf("calls = %d, want 4 (500, 500, 200, then 0)", calls)
	}
}

func TestMoveJobsStopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rekey := func(context.Context, string, string, int) (int64, error) {
		cancel()
		return 500, nil
	}
	moved, err := MoveJobs(ctx, rekey, "a", "b", 500)
	if moved != 500 || !errors.Is(err, context.Canceled) {
		t.Errorf("moved %d, err %v; want 500 and context.Canceled", moved, err)
	}
}
