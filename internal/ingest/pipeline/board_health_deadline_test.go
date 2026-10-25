package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// ctxHealth refuses a write on a context that has ended, as the database driver does — the
// behaviour that lost a board's health row when the crawl met its deadline.
type ctxHealth struct {
	fakeHealth
	failureAttempts int
}

func (h *ctxHealth) RecordSuccess(ctx context.Context, provider, board, region string, ingested int, reached bool, fingerprint string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.fakeHealth.RecordSuccess(ctx, provider, board, region, ingested, reached, fingerprint)
}

func (h *ctxHealth) RecordFailure(ctx context.Context, provider, board, region, msg string) error {
	h.failureAttempts++
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.fakeHealth.RecordFailure(ctx, provider, board, region, msg)
}

// stoppingSource ends the crawl's context while the board is being fetched, standing in for
// cmd/ingest's crawl deadline arriving mid-board.
type stoppingSource struct {
	fakeSource
	stop context.CancelFunc
}

func (d stoppingSource) Fetch(ctx context.Context, e sources.CompanyEntry) ([]sources.Job, error) {
	d.stop()
	return d.fakeSource.Fetch(ctx, e)
}

func runPastDeadline(t *testing.T, src fakeSource) *ctxHealth {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	health := &ctxHealth{}
	r := Runner{Registry: registry(stoppingSource{src, cancel}), Store: &fakeStore{}, BoardHealth: health}
	_, _ = r.Run(ctx, []sources.CompanyEntry{{Company: "Acme", Provider: src.provider, Board: "acme"}})
	return health
}

// A board that answered before the deadline is recorded as a success, though the crawl's
// context has ended by the time its outcome is written.
func TestBoardHealthRecordsSuccessPastTheDeadline(t *testing.T) {
	health := runPastDeadline(t, fakeSource{provider: "greenhouse", jobs: []sources.Job{
		{ExternalID: "1", Title: "Backend Engineer", Company: "Acme"},
	}})
	if len(health.successes) != 1 || health.successes[0] != "greenhouse/acme/" {
		t.Errorf("successes = %v, want the board recorded", health.successes)
	}
	if !health.reached["greenhouse/acme/"] {
		t.Error("reached = false, want true: the listing named a posting")
	}
}

// A failure that arrives after the deadline cannot be told apart from the deadline cutting the
// fetch short, so it is not counted against the board.
func TestBoardHealthIgnoresAFailurePastTheDeadline(t *testing.T) {
	health := runPastDeadline(t, fakeSource{provider: "greenhouse", err: context.DeadlineExceeded})
	if health.failureAttempts != 0 || len(health.successes) != 0 {
		t.Errorf("%d failure writes attempted, successes = %v, want nothing recorded", health.failureAttempts, health.successes)
	}
}

// The control: before the deadline a failed fetch still counts.
func TestBoardHealthRecordsAFailureBeforeTheDeadline(t *testing.T) {
	health := &ctxHealth{}
	r := Runner{Registry: registry(fakeSource{provider: "greenhouse", err: errors.New("boom")}), Store: &fakeStore{}, BoardHealth: health}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Company: "Acme", Provider: "greenhouse", Board: "acme"}}); err != nil {
		t.Fatal(err)
	}
	if len(health.failures) != 1 {
		t.Errorf("failures = %v, want the board recorded", health.failures)
	}
}
