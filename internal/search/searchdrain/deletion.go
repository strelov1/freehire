package searchdrain

import (
	"context"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/platform/outbox"
)

// DeletionStore is the persistence the deletion runner needs — the same lease/retry shape as
// Store, minus Jobs and Reap.
//
// There is no Jobs: a removal identifies its document by primary key, and by the time an
// entry is drained the job row is often gone (cmd/prune hard-deletes). That is the ordinary
// case here, not a corruption to route around.
//
// There is no Reap either. Store.Reap exists because the indexing claim skips entries whose
// job closed or became a repost, which strands them forever. This claim skips nothing, so
// nothing strands; dead-lettered entries are cleaned on their own schedule.
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
	Deleted      int
	Failed       int
	DeadLettered int
}

// DeletionRunner drains the removal queue wave by wave until no claimable entries remain.
//
// It is a sibling of Runner rather than a phase inside it: the two share a queue shape and an
// index, but nothing else. This one builds no documents, loads no rows, and reaps nothing, so
// folding it in would mean a Runner whose Store half-applies to half its work.
type DeletionRunner struct {
	Store   DeletionStore
	Deleter Deleter
}

// Run drains the removal queue. A failure on a single entry is recorded and never aborts the
// run.
func (r DeletionRunner) Run(ctx context.Context, opt RunOptions) (DeletionStats, error) {
	rn := &deletionRun{store: r.Store, deleter: r.Deleter, opt: opt}

	_, err := outbox.RunBatch(ctx, claimFunc(r.Store.Claim), outbox.RunOptions{
		BatchSize:    opt.BatchSize,
		LeaseSeconds: opt.LeaseSeconds,
		MaxPerRun:    opt.MaxPerRun,
		OnWave: func(outbox.Stats) {
			log.Printf("search-drain: progress deleted=%d failed=%d dead=%d",
				rn.stats.Deleted, rn.stats.Failed, rn.stats.DeadLettered)
		},
	}, rn.processWave, rn.unreachableFallback)
	if err != nil {
		return rn.stats, fmt.Errorf("claim deletions: %w", err)
	}
	return rn.stats, nil
}

// claimFunc adapts a bare Claim method to outbox.Claimer, which the indexing Store satisfies
// wholesale only because it happens to carry the other methods too.
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
	return outbox.Stats{
		Succeeded:    rn.stats.Deleted - before.Deleted,
		Failed:       rn.stats.Failed - before.Failed,
		DeadLettered: rn.stats.DeadLettered - before.DeadLettered,
	}, nil
}

// unreachableFallback satisfies outbox.RunBatch's Processor parameter. processWave never
// returns an error, so this is never called; panicking documents the invariant rather than
// fabricating an outcome.
func (rn *deletionRun) unreachableFallback(context.Context, Claimed) outbox.Outcome {
	panic("search-drain: outer per-item fallback invoked, but processWave never returns an error")
}

// processBatch removes a whole wave as one index task, falling back to per-item removal if
// that fails — with one exception, mirroring the indexing runner.
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

	// A call context that ended is not a per-document defect: the wave is skipped whole and
	// left claimed, so its lease expiry retries it fresh. Falling back here would turn one
	// slow call into BatchSize equally slow ones, all competing for the same disk — the
	// shape that produced a real outage on the indexing side (see the incident note in
	// AGENTS.md).
	if ctxErr := callCtx.Err(); ctxErr != nil {
		log.Printf("search-drain: deletion wave of %d abandoned (%v), skipping (lease will retry): %v",
			len(entries), ctxErr, err)
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
		// Same rule as the wave above, one entry down: a call that was abandoned taught
		// us nothing about this entry, so it keeps its attempt budget and its claim.
		if ctxErr := callCtx.Err(); ctxErr != nil {
			log.Printf("search-drain: deletion for job %d abandoned (%v), leaving claimed: %v",
				entry.JobID, ctxErr, err)
			return
		}
		dead, failErr := rn.store.Fail(ctx, entry.OutboxID, err.Error(), rn.opt.MaxAttempts)
		if failErr != nil {
			log.Printf("search-drain: record deletion failure for job %d: %v", entry.JobID, failErr)
		}
		// One entry, one outcome — dead-lettered OR failed, never both, matching the
		// indexing runner's failN. Counting it twice made a single exhausted entry read
		// as two problems in the run's own summary line and in the exit code's inputs.
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

// callContext bounds one index operation, or returns the parent unchanged when no timeout is
// configured.
//
// Whether the call context ended is the whole classification, so the two branches must not
// differ in whether one exists: the untimed branch returns the parent, which is exactly
// what "the run is being cancelled" needs it to report.
func (rn *deletionRun) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if rn.opt.CallTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, rn.opt.CallTimeout)
}
