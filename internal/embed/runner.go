// Package embed drives incremental semantic embedding: enqueue open jobs whose vector
// is missing/stale (and closed jobs whose vector must be removed), then drain the
// semantic_outbox queue wave by wave. Each wave is embedded and upserted as ONE batch
// (one Meilisearch task per wave, not per job) so a bulk backfill isn't bottlenecked on
// Meili's serial task queue; on a batch failure it falls back to per-item processing so
// a single poison/corrupted row can't sink the whole batch. It mirrors internal/enrich:
// the Runner is independent of the DB and search layers (Store + Indexer ports), so the
// batch/fallback logic is unit-tested with fakes; cmd/embed wires the real adapters.
package embed

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

// Claimed is one outbox entry leased to this run. Closed marks whether the job is now
// unindexable — closed OR a non-canonical repost (duplicate_of set): open canonical jobs
// are embedded, unindexable ones have their document removed.
type Claimed struct {
	OutboxID int64
	JobID    int64
	Closed   bool
}

// Store is the persistence the runner needs, in domain terms so the runner is
// independent of the DB layer. The real implementation wraps the generated queries and
// a pool (running CompleteOpen/CompleteClosed in a transaction); tests use a fake.
type Store interface {
	// Enqueue adds outbox entries for jobs needing (re-)embedding or removal at model.
	Enqueue(ctx context.Context, targetModel string) (int64, error)
	// Claim leases up to batch live, unleased entries (closed jobs included).
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Jobs returns the persisted rows the documents are built from. A corrupted row
	// aborts the whole load, so the runner retries such a batch per item to isolate it.
	Jobs(ctx context.Context, ids []int64) ([]db.Job, error)
	// CompleteOpen stamps each entry's job embed provenance (model + its current
	// content_hash), persists each job's semantic vector, and deletes the outbox
	// entries, atomically. vectors maps job id to the vector just upserted into the
	// index, so the durable Postgres copy commits with the provenance stamp.
	CompleteOpen(ctx context.Context, entries []Claimed, model string, vectors map[int64][]float32) error
	// CompleteClosed clears each entry's job embed provenance and deletes the outbox
	// entries, atomically (their documents were just removed from the index).
	CompleteClosed(ctx context.Context, entries []Claimed) error
	// Fail records a failed attempt for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
}

// Indexer is the semantic-index side: embed+upsert open jobs, or remove closed ones.
type Indexer interface {
	// IndexOpen embeds the jobs' documents and upserts their vectors into the semantic
	// index in one batch, returning the vectors keyed by job id so they can be persisted
	// to Postgres alongside the provenance stamp.
	IndexOpen(ctx context.Context, jobs []db.Job) (map[int64][]float32, error)
	// RemoveClosed deletes the jobs' documents from the semantic index in one batch.
	RemoveClosed(ctx context.Context, ids []int64) error
}

// RunOptions are the per-run knobs.
type RunOptions struct {
	// TargetModel is the embedder identity: the enqueue staleness key and the value
	// stamped on a successful embed (search.CurrentEmbedderModel()).
	TargetModel string
	// BatchSize is the claim wave size and the embed/upsert batch size — the lever that
	// collapses per-doc Meili tasks into one task per wave. The embed backend chunks the
	// batch internally (EMBED_CONCURRENCY), so this can be large (hundreds).
	BatchSize    int
	LeaseSeconds int
	MaxAttempts  int
	// CallTimeout bounds a single batch's (or fallback item's) index/remove operation;
	// 0 means no per-call timeout (the embed backend has its own per-attempt timeout).
	CallTimeout time.Duration
}

// Stats reports what a run did.
type Stats struct {
	Indexed      int
	Removed      int
	Failed       int
	DeadLettered int
}

// Runner drives the process: enqueue outstanding work, then drain claimed waves.
type Runner struct {
	Store   Store
	Indexer Indexer
}

// Run enqueues outstanding jobs and drains the queue until no claimable entries remain.
// A failure on a single entry is recorded and never aborts the run.
func (r Runner) Run(ctx context.Context, opt RunOptions) (Stats, error) {
	enqueued, err := r.Store.Enqueue(ctx, opt.TargetModel)
	if err != nil {
		return Stats{}, fmt.Errorf("enqueue: %w", err)
	}
	log.Printf("embed: enqueued %d pending, draining (batch=%d)", enqueued, opt.BatchSize)

	rn := &run{store: r.Store, indexer: r.Indexer, opt: opt}
	for {
		batch, err := r.Store.Claim(ctx, opt.BatchSize, opt.LeaseSeconds)
		if err != nil {
			return rn.stats, fmt.Errorf("claim: %w", err)
		}
		if len(batch) == 0 {
			return rn.stats, nil
		}
		var open, closed []Claimed
		for _, e := range batch {
			if e.Closed {
				closed = append(closed, e)
			} else {
				open = append(open, e)
			}
		}
		rn.processOpenBatch(ctx, open)
		rn.processClosedBatch(ctx, closed)
		log.Printf("embed: progress indexed=%d removed=%d failed=%d dead=%d",
			rn.stats.Indexed, rn.stats.Removed, rn.stats.Failed, rn.stats.DeadLettered)
	}
}

// run accumulates one Run's options and tallies. Waves are processed sequentially (the
// embed concurrency lives inside the Indexer), so the tallies need no lock.
type run struct {
	store   Store
	indexer Indexer
	opt     RunOptions
	stats   Stats
}

// processOpenBatch embeds+upserts a whole wave of open jobs in one batch and completes
// them in one transaction. Any batch-level failure (a corrupted-row load, a batch embed
// error, a partial load) falls back to per-item processing so one bad entry can't sink
// the wave.
func (rn *run) processOpenBatch(ctx context.Context, entries []Claimed) {
	if len(entries) == 0 {
		return
	}
	start := time.Now()
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	jobs, err := rn.store.Jobs(callCtx, jobIDs(entries))
	if err != nil || len(jobs) != len(entries) {
		rn.fallbackOpen(ctx, entries)
		return
	}
	vectors, err := rn.indexer.IndexOpen(callCtx, jobs)
	if err != nil {
		rn.fallbackOpen(ctx, entries)
		return
	}
	if err := rn.store.CompleteOpen(callCtx, entries, rn.opt.TargetModel, vectors); err != nil {
		rn.fallbackOpen(ctx, entries)
		return
	}
	rn.stats.Indexed += len(entries)
	log.Printf("embed: indexed batch of %d in %s", len(entries), since(start))
}

func (rn *run) fallbackOpen(ctx context.Context, entries []Claimed) {
	for _, e := range entries {
		rn.processOpenOne(ctx, e)
	}
}

func (rn *run) processOpenOne(ctx context.Context, entry Claimed) {
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	jobs, err := rn.store.Jobs(callCtx, []int64{entry.JobID})
	if err != nil {
		// A corrupted row (XX001) can never load — dead-letter it immediately rather
		// than burning the attempt budget across cron runs (mirrors enrich).
		if worker.IsCorruptedRow(err) {
			rn.failN(entry, fmt.Errorf("load job: %w", err), 1)
			return
		}
		rn.fail(entry, fmt.Errorf("load job: %w", err))
		return
	}
	if len(jobs) == 0 {
		rn.fail(entry, fmt.Errorf("job %d not found", entry.JobID))
		return
	}
	vectors, err := rn.indexer.IndexOpen(callCtx, jobs)
	if err != nil {
		rn.fail(entry, fmt.Errorf("embed/index: %w", err))
		return
	}
	if err := rn.store.CompleteOpen(callCtx, []Claimed{entry}, rn.opt.TargetModel, vectors); err != nil {
		rn.fail(entry, fmt.Errorf("complete open: %w", err))
		return
	}
	rn.stats.Indexed++
}

// processClosedBatch removes a whole wave of closed jobs' documents in one batch, with
// the same per-item fallback on failure.
func (rn *run) processClosedBatch(ctx context.Context, entries []Claimed) {
	if len(entries) == 0 {
		return
	}
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	if err := rn.indexer.RemoveClosed(callCtx, jobIDs(entries)); err != nil {
		rn.fallbackClosed(ctx, entries)
		return
	}
	if err := rn.store.CompleteClosed(callCtx, entries); err != nil {
		rn.fallbackClosed(ctx, entries)
		return
	}
	rn.stats.Removed += len(entries)
}

func (rn *run) fallbackClosed(ctx context.Context, entries []Claimed) {
	for _, e := range entries {
		rn.processClosedOne(ctx, e)
	}
}

func (rn *run) processClosedOne(ctx context.Context, entry Claimed) {
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	if err := rn.indexer.RemoveClosed(callCtx, []int64{entry.JobID}); err != nil {
		rn.fail(entry, fmt.Errorf("remove closed: %w", err))
		return
	}
	if err := rn.store.CompleteClosed(callCtx, []Claimed{entry}); err != nil {
		rn.fail(entry, fmt.Errorf("complete closed: %w", err))
		return
	}
	rn.stats.Removed++
}

// callContext derives the per-batch timeout context (no-op when CallTimeout is 0).
func (rn *run) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if rn.opt.CallTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, rn.opt.CallTimeout)
}

func (rn *run) fail(entry Claimed, cause error) {
	rn.failN(entry, cause, rn.opt.MaxAttempts)
}

// failN records a failure with an explicit attempt ceiling. fail uses the run's
// MaxAttempts; the corrupted-row path passes 1 to force an immediate dead-letter.
func (rn *run) failN(entry Claimed, cause error, maxAttempts int) {
	// Fail bookkeeping runs on the run's background context, not the per-call one:
	// a timed-out/cancelled call must still record its own failure.
	dead, err := rn.store.Fail(context.Background(), entry.OutboxID, cause.Error(), maxAttempts)
	if err != nil {
		log.Printf("embed: outbox=%d fail-bookkeeping error: %v", entry.OutboxID, err)
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
