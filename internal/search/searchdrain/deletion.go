package searchdrain

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/platform/outbox"
)

// DeletionStore is the persistence the deletion runner needs: Store's lease/retry shape minus Jobs and Reap.
// There is no Jobs because a removal is identified by primary key and the job row may already be hard-deleted.
// There is no Reap because this claim skips nothing, so nothing strands.
type DeletionStore interface {
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	Complete(ctx context.Context, entries []Claimed) error
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
}

// Deleter removes documents from the live facet index by job id.
type Deleter interface {
	// DeleteBatch removes the whole wave as one index task. Removing an id that is not
	// indexed is a no-op, so a wave of ids that were never indexed still succeeds.
	DeleteBatch(ctx context.Context, jobIDs []int64) error
}

// DeletionStats reports what a deletion run did.
type DeletionStats struct {
	// Claimed includes waves deferred for lease-expiry retry.
	Claimed      int
	Deleted      int
	Failed       int
	DeadLettered int
}

// DeletionRunner drains the removal queue wave by wave until no claimable entries remain.
// It is a sibling of Runner, not a phase in it: they share only the queue shape and the index.
type DeletionRunner struct {
	Store   DeletionStore
	Deleter Deleter
}

// Run drains the removal queue. A failure on a single entry is recorded and never aborts the
// run.
func (r DeletionRunner) Run(ctx context.Context, opt RunOptions) (DeletionStats, error) {
	return r.run(ctx, opt, false)
}

func (r DeletionRunner) run(ctx context.Context, opt RunOptions, oneWave bool) (DeletionStats, error) {
	rn := &deletionRun{store: r.Store, deleter: r.Deleter, opt: opt}

	_, err := outbox.RunBatch(ctx, claimFunc(func(ctx context.Context, batch, lease int) ([]Claimed, error) {
		if oneWave && rn.stats.Claimed > 0 {
			return nil, nil
		}
		entries, err := r.Store.Claim(ctx, batch, lease)
		rn.stats.Claimed += len(entries)
		return entries, err
	}), outbox.RunOptions{
		BatchSize:    opt.BatchSize,
		LeaseSeconds: opt.LeaseSeconds,
		MaxPerRun:    opt.MaxPerRun,
		OnWave: func(outbox.Stats) {
			if !oneWave {
				log.Printf("search-drain: progress deleted=%d failed=%d dead=%d",
					rn.stats.Deleted, rn.stats.Failed, rn.stats.DeadLettered)
			}
		},
	}, rn.processWave, rn.unreachableFallback)
	if err != nil {
		return rn.stats, fmt.Errorf("claim deletions: %w", err)
	}
	return rn.stats, nil
}

// claimFunc adapts a bare Claim method to outbox.Claimer.
type claimFunc func(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)

func (f claimFunc) Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
	return f(ctx, batch, leaseSeconds)
}

type deletionRun struct {
	store   DeletionStore
	deleter Deleter
	opt     RunOptions
	stats   DeletionStats
}

func (rn *deletionRun) processWave(ctx context.Context, batch []Claimed) (outbox.Stats, error) {
	before := rn.stats
	rn.processBatch(ctx, batch)
	delta := outbox.Stats{
		Succeeded:    rn.stats.Deleted - before.Deleted,
		Failed:       rn.stats.Failed - before.Failed,
		DeadLettered: rn.stats.DeadLettered - before.DeadLettered,
	}
	delta.Deferred = len(batch) - delta.Succeeded - delta.Failed - delta.DeadLettered
	return delta, nil
}

// unreachableFallback satisfies outbox.RunBatch; processWave never errors, so it is never called.
func (rn *deletionRun) unreachableFallback(context.Context, Claimed) outbox.Outcome {
	panic("search-drain: outer per-item fallback invoked, but processWave never returns an error")
}

// processBatch removes a wave as one index task, falling back to per-item removal if that fails.
func (rn *deletionRun) processBatch(ctx context.Context, entries []Claimed) {
	if len(entries) == 0 {
		return
	}
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	err := rn.deleter.DeleteBatch(callCtx, jobIDs(entries))
	if err == nil {
		if err := rn.store.Complete(ctx, entries); err != nil {
			log.Printf("search-drain: complete deletion wave: %v", err)
			return
		}
		rn.stats.Deleted += len(entries)
		return
	}

	// An ended call context is not a per-document defect: leave the wave claimed so lease expiry
	// retries it. Falling back would turn one slow call into BatchSize slow ones.
	if ctxErr := callCtx.Err(); ctxErr != nil {
		log.Printf("search-drain: deletion wave of %d abandoned (%v), skipping (lease will retry): %v",
			len(entries), ctxErr, err)
		return
	}

	if errors.Is(err, ErrSkipWave) {
		log.Printf("search-drain: deletion wave of %d left claimed for lease-expiry retry: %v", len(entries), err)
		return
	}

	log.Printf("search-drain: deletion wave of %d failed, falling back per item: %v", len(entries), err)
	for _, entry := range entries {
		rn.processOne(ctx, entry)
	}
}

func (rn *deletionRun) processOne(ctx context.Context, entry Claimed) {
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	if err := rn.deleter.DeleteBatch(callCtx, []int64{entry.JobID}); err != nil {
		// Same rule as the wave: an abandoned call keeps the entry's attempt budget and claim.
		if ctxErr := callCtx.Err(); ctxErr != nil {
			log.Printf("search-drain: deletion for job %d abandoned (%v), leaving claimed: %v",
				entry.JobID, ctxErr, err)
			return
		}
		if errors.Is(err, ErrSkipWave) {
			log.Printf("search-drain: deletion for job %d left claimed for lease-expiry retry: %v", entry.JobID, err)
			return
		}
		dead, failErr := rn.store.Fail(ctx, entry.OutboxID, err.Error(), rn.opt.MaxAttempts)
		if failErr != nil {
			log.Printf("search-drain: record deletion failure for job %d: %v", entry.JobID, failErr)
		}
		// One outcome per entry: dead-lettered or failed, never both.
		if failErr == nil && dead {
			rn.stats.DeadLettered++
			return
		}
		rn.stats.Failed++
		return
	}
	if err := rn.store.Complete(ctx, []Claimed{entry}); err != nil {
		log.Printf("search-drain: complete deletion for job %d: %v", entry.JobID, err)
		return
	}
	rn.stats.Deleted++
}

// callContext bounds one index operation, or returns the parent unchanged when no timeout is set.
// The untimed branch returns the parent so cancellation still reads as an ended context.
func (rn *deletionRun) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if rn.opt.CallTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, rn.opt.CallTimeout)
}
