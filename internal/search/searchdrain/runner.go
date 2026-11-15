// Package searchdrain drives the incremental facet-search queue: it drains search_outbox
// wave by wave into the live Meilisearch `jobs` index, one Meili task per wave.
//
// A push costs about the same whatever its size (Meilisearch re-merges its structures across
// the whole index), so all writes go through one outbox and one worker. Unlike internal/ai/embed
// there is no open/closed split: a job that closed or became a non-canonical repost after being
// queued is not claimed (see ClaimSearchOutboxBatch). Such entries are never purged by a reindex
// (ingest keeps touching the row), so each run reaps them first (Store.Reap).
package searchdrain

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/outbox"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Claimed is one outbox entry leased to this run.
type Claimed struct {
	OutboxID int64
	JobID    int64
}

// Store is the persistence the runner needs. The real implementation wraps the
// generated queries and a pool; tests use a fake.
type Store interface {
	// Claim leases up to batch live, unleased entries.
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Jobs returns the persisted rows the documents are built from. A corrupted row
	// aborts the whole load, so the runner retries such a batch per item to isolate it.
	Jobs(ctx context.Context, ids []int64) ([]db.Job, error)
	// Complete deletes the outbox entries for a wave that was successfully indexed.
	Complete(ctx context.Context, entries []Claimed) error
	// Fail records a failed attempt for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
	// Reap deletes up to maxRows live entries Claim can never take (job closed, repost, or gone),
	// returning how many it removed.
	Reap(ctx context.Context, maxRows int) (int64, error)
}

// Indexer pushes a batch of jobs into the live facet index.
type Indexer interface {
	// IndexBatch builds and pushes the jobs' documents as one Meili task.
	IndexBatch(ctx context.Context, jobs []db.Job) error
}

// ErrSkipWave is what an Indexer returns when it could not assemble the wave's documents
// completely because something every document needs was unavailable.
// The runner leaves such a wave claimed until its lease expires instead of falling back per
// item: a push is a field-level update, so an incomplete document would overwrite what is
// already indexed.
var ErrSkipWave = errors.New("searchdrain: wave left claimed for lease-expiry retry")

// RunOptions are the per-run knobs.
type RunOptions struct {
	// BatchSize is the claim wave size and the index-push batch size.
	BatchSize    int
	LeaseSeconds int
	MaxAttempts  int
	// Stop, when set, is asked before each wave of Drain; true ends the run cleanly.
	Stop func() bool
	// CallTimeout bounds a single batch's (or fallback item's) index operation; 0
	// means no per-call timeout.
	CallTimeout time.Duration
	// MaxPerRun independently bounds claimed entries from each queue; 0 is
	// unbounded. Deferred waves consume this budget without spending attempts.
	MaxPerRun int
}

// Stats reports what a run did.
type Stats struct {
	// Claimed includes waves deferred for lease-expiry retry.
	Claimed      int
	Indexed      int
	Failed       int
	DeadLettered int
	// Reaped counts entries deleted because Claim could never take them (housekeeping).
	Reaped int
}

// reapLimit bounds how many ineligible entries one run deletes, so a large backlog cannot turn
// a drain tick into a long delete holding row locks the claim waits on.
const reapLimit = 2000

// Runner drains the queue wave by wave until no claimable entries remain.
type Runner struct {
	Store   Store
	Indexer Indexer
}

// Run drains the queue until no claimable entries remain. A failure on a single
// entry is recorded and never aborts the run.
func (r Runner) Run(ctx context.Context, opt RunOptions) (Stats, error) {
	return r.run(ctx, opt, false)
}

func (r Runner) run(ctx context.Context, opt RunOptions, oneWave bool) (Stats, error) {
	rn := &run{store: r.Store, indexer: r.Indexer, opt: opt}

	if !oneWave {
		rn.stats.Reaped = reap(ctx, r.Store)
	}

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
			// Heartbeat per wave so a long drain does not go silent.
			if !oneWave {
				log.Printf("search-drain: progress indexed=%d failed=%d dead=%d",
					rn.stats.Indexed, rn.stats.Failed, rn.stats.DeadLettered)
			}
		},
	}, rn.processWave, rn.unreachableFallback)
	if err != nil {
		return rn.stats, fmt.Errorf("claim: %w", err)
	}
	return rn.stats, nil
}

// reap bounds housekeeping across the whole invocation, not each cooperative wave.
func reap(ctx context.Context, store Store) int {
	n, err := store.Reap(ctx, reapLimit)
	if err != nil {
		log.Printf("search-drain: reap ineligible entries (continuing): %v", err)
		return 0
	}
	if n > 0 {
		log.Printf("search-drain: reaped %d entries whose job closed or became a repost", n)
	}
	return int(n)
}

// run accumulates one Run's options and tallies. Waves are sequential, so no lock.
type run struct {
	store   Store
	indexer Indexer
	opt     RunOptions
	stats   Stats
}

// processWave is search-drain's outbox.BatchProcessor: it runs processBatch and reports the
// wave's delta. It never returns an error, so unreachableFallback is never invoked.
func (rn *run) processWave(ctx context.Context, batch []Claimed) (outbox.Stats, error) {
	before := rn.stats
	rn.processBatch(ctx, batch)
	delta := outbox.Stats{
		Succeeded:    rn.stats.Indexed - before.Indexed,
		Failed:       rn.stats.Failed - before.Failed,
		DeadLettered: rn.stats.DeadLettered - before.DeadLettered,
	}
	delta.Deferred = len(batch) - delta.Succeeded - delta.Failed - delta.DeadLettered
	return delta, nil
}

// unreachableFallback satisfies outbox.RunBatch's Processor parameter; processWave never errors,
// so it is never called.
func (rn *run) unreachableFallback(context.Context, Claimed) outbox.Outcome {
	panic("search-drain: outer per-item fallback invoked, but processWave never returns an error")
}

// processBatch indexes a whole wave in one batch and completes it in one call. Any batch-level
// failure splits the wave (see split) so one bad entry cannot sink it.
func (rn *run) processBatch(ctx context.Context, entries []Claimed) {
	if len(entries) == 0 {
		return
	}
	start := time.Now()
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	jobs, err := rn.store.Jobs(callCtx, jobIDs(entries))
	if err != nil {
		if rn.skipOnContextEnd(callCtx, entries, "load jobs") {
			return
		}
		rn.split(ctx, entries, fmt.Errorf("load jobs: %w", err))
		return
	}
	if len(jobs) != len(entries) {
		rn.split(ctx, entries, fmt.Errorf("loaded %d of %d jobs", len(jobs), len(entries)))
		return
	}
	if err := rn.indexer.IndexBatch(callCtx, jobs); err != nil {
		if rn.skipOnContextEnd(callCtx, entries, "index") {
			return
		}
		if errors.Is(err, ErrSkipWave) {
			log.Printf("search-drain: batch of %d not indexed, leaving claimed for "+
				"lease-expiry retry: %v", len(entries), err)
			return
		}
		rn.split(ctx, entries, fmt.Errorf("index: %w", err))
		return
	}
	if err := rn.store.Complete(callCtx, entries); err != nil {
		// The documents are already indexed and Complete is one DELETE that cannot fail on a row's
		// content, so a per-item fallback would only re-push every document. Leave the wave claimed
		// for lease expiry; the later re-push is idempotent.
		log.Printf("search-drain: complete indexed batch of %d (leaving claimed for "+
			"lease-expiry retry): %v", len(entries), err)
		return
	}
	rn.stats.Indexed += len(entries)
	log.Printf("search-drain: indexed batch of %d in %s", len(entries), since(start))
}

// skipOnContextEnd reports whether callCtx ended before the stage finished (CallTimeout or run
// cancellation) and, if so, logs and leaves the wave claimed so a later run retries it whole.
// It classifies on the context, not the error: Meilisearch's *Error has no Unwrap, so errors.Is
// against a context sentinel is blind to it.
// Falling back per item on a timeout would turn one slow batch into len(entries) equally slow
// calls. On cancellation it is wrong: reindexw stops this service before every rebuild, and each
// item's Fail reaches Postgres on a background context, so three stops would set failed_at and
// the entry would never be re-enqueued (ON CONFLICT DO NOTHING) until the next full rebuild.
func (rn *run) skipOnContextEnd(callCtx context.Context, entries []Claimed, stage string) bool {
	cause := callCtx.Err()
	if cause == nil {
		return false
	}
	log.Printf("search-drain: batch of %d abandoned during %s (%v, budget %s) — leaving claimed "+
		"for lease-expiry retry, not falling back to per-item", len(entries), stage, cause, rn.opt.CallTimeout)
	return true
}

// split retries a failed batch as two halves, each through processBatch, down to single
// entries that processOne isolates and charges.
// It halves instead of going per item because a push costs about the same whatever its size:
// a wave rejected as a whole (e.g. payload over Meilisearch's HTTP limit, 413) needs a split or
// two, and one poison row among N costs ~2*log2(N) pushes against N per item.
func (rn *run) split(ctx context.Context, entries []Claimed, cause error) {
	if len(entries) == 1 {
		rn.processOne(ctx, entries[0])
		return
	}
	log.Printf("search-drain: batch of %d failed, retrying as two halves: %v", len(entries), cause)
	mid := len(entries) / 2
	for _, half := range [][]Claimed{entries[:mid], entries[mid:]} {
		if len(half) == 1 {
			rn.processOne(ctx, half[0])
			continue
		}
		rn.processBatch(ctx, half)
	}
}

func (rn *run) processOne(ctx context.Context, entry Claimed) {
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	jobs, err := rn.store.Jobs(callCtx, []int64{entry.JobID})
	if err != nil {
		// A corrupted row (XX001) can never load: dead-letter it now instead of burning attempts.
		if pgerr.IsDataCorrupted(err) {
			rn.failN(entry, fmt.Errorf("load job: %w", err), 1)
			return
		}
		rn.fail(callCtx, entry, fmt.Errorf("load job: %w", err))
		return
	}
	if len(jobs) == 0 {
		// A missing row (hard-deleted, closed, or a repost) is as deterministic as a corrupted one:
		// dead-letter it now.
		rn.failN(entry, fmt.Errorf("job %d not found", entry.JobID), 1)
		return
	}
	if err := rn.indexer.IndexBatch(callCtx, jobs); err != nil {
		rn.fail(callCtx, entry, fmt.Errorf("index: %w", err))
		return
	}
	if err := rn.store.Complete(callCtx, []Claimed{entry}); err != nil {
		rn.fail(callCtx, entry, fmt.Errorf("complete: %w", err))
		return
	}
	rn.stats.Indexed++
}

// callContext derives the per-batch timeout context (a no-op when CallTimeout is 0).
func (rn *run) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if rn.opt.CallTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, rn.opt.CallTimeout)
}

// fail records a failed attempt against one entry, unless the call context ended first: an
// abandoned wave (CallTimeout, or SIGTERM before a rebuild) must not be charged an attempt,
// or three ordinary stops would dead-letter a good job that nothing re-adds.
func (rn *run) fail(callCtx context.Context, entry Claimed, cause error) {
	if ctxErr := callCtx.Err(); ctxErr != nil {
		log.Printf("search-drain: outbox=%d abandoned (%v), leaving claimed for lease-expiry "+
			"retry: %v", entry.OutboxID, ctxErr, cause)
		return
	}
	// ErrSkipWave means the entry is not at fault (a dependency the indexer needed was
	// unreadable). Honour it here too, not only in processBatch: a batch that fell back for another
	// reason would meet the same dependency per entry and burn a healthy row's attempt budget.
	if errors.Is(cause, ErrSkipWave) {
		log.Printf("search-drain: outbox=%d not indexed, leaving claimed for lease-expiry "+
			"retry: %v", entry.OutboxID, cause)
		return
	}
	rn.failN(entry, cause, rn.opt.MaxAttempts)
}

// failN records a failure with an explicit attempt ceiling; the corrupted/missing-row paths
// pass 1 to dead-letter immediately.
func (rn *run) failN(entry Claimed, cause error, maxAttempts int) {
	// Runs on a background context so a timed-out or cancelled call still records its failure.
	dead, err := rn.store.Fail(context.Background(), entry.OutboxID, cause.Error(), maxAttempts)
	if err != nil {
		log.Printf("search-drain: outbox=%d fail-bookkeeping error: %v", entry.OutboxID, err)
	}
	if err == nil && dead {
		rn.stats.DeadLettered++
		return
	}
	rn.stats.Failed++
}

func jobIDs(entries []Claimed) []int64 {
	ids := make([]int64, len(entries))
	for i, e := range entries {
		ids[i] = e.JobID
	}
	return ids
}

func since(t time.Time) time.Duration { return time.Since(t).Round(time.Millisecond) }
