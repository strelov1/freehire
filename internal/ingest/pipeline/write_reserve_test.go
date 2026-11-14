package pipeline

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
)

// deadlineAwareStore refuses a write on an ended context, the way the database driver does.
type deadlineAwareStore struct{ *fakeStore }

func (s deadlineAwareStore) Save(ctx context.Context, j job.Job) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.fakeStore.Save(ctx, j)
}

// A board whose fetch the deadline catches returns what it read, and until 2026-11 every one of
// those postings then failed to save on the same ended context: Lowe's, Costco and Domino's
// boards (10k-25k postings) saved nothing run after run, and each run started them from zero.
// WriteReserve ends the fetches that much ahead of the run's deadline, so what a cut board read
// is written, and the next run starts past it. The board is still cut short, so it must stay
// interrupted and out of the board-scoped close.
func TestRunWritesWhatACutBoardReadWithinTheWriteReserve(t *testing.T) {
	store := deadlineAwareStore{&fakeStore{}}
	r := Runner{Registry: registry(deadlineSource{provider: "workday"}), Store: store, WriteReserve: 300 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	stats, err := r.Run(ctx, []sources.CompanyEntry{
		{Company: "Fast Co", Provider: "workday", Board: "fast"},
		{Company: "Slow Co", Provider: "workday", Board: "slow"},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want one wrapping the deadline: the run did not finish every board", err)
	}
	if got := len(store.saved); got != 2 {
		t.Fatalf("saved %d postings, want 2: the cut board's read must be written", got)
	}
	got := stats["workday"]
	if got.Interrupted != 1 || got.Failed != 0 || got.Skipped != 0 {
		t.Errorf("Interrupted = %d, Failed = %d, Skipped = %d; want 1, 0, 0", got.Interrupted, got.Failed, got.Skipped)
	}
	if names := boardNames(got.QualifyingBoards); !slices.Equal(names, []string{"fast"}) {
		t.Errorf("QualifyingBoards = %v, want only the board finished in time", names)
	}
}
