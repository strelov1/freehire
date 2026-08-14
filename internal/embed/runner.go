// Package embed drives incremental semantic embedding: enqueue open jobs whose vector
// is missing/stale (and closed jobs whose vector must be removed), then drain the
// semantic_outbox queue wave by wave. Each wave is embedded and upserted as ONE batch
// (one Meilisearch task per wave, not per job) so a bulk backfill isn't bottlenecked on
// Meili's serial task queue; on a batch failure it falls back to per-item processing so
// a single poison/corrupted row can't sink the whole batch. It mirrors internal/enrich:
// the batch/fallback logic is unit-tested with fakes of the Store + Indexer ports and
// cmd/embed wires the real adapters. The ports trade in db.Job rows (not a domain
// type), so the runner is independent of the pool/queries, not of the generated row
// shape.
package embed

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/outbox"
	"github.com/strelov1/freehire/internal/pgerr"
)

// Claimed is one outbox entry leased to this run. Closed marks whether the job is now
// unindexable — closed OR a non-canonical repost (duplicate_of set): open canonical jobs
// are embedded, unindexable ones have their document removed.
type Claimed struct {
	OutboxID int64
	JobID    int64
	Closed   bool
}

// ChunkEmbedding is one embedding vector for one 0-indexed chunk of a job's
// description — the pgvector-backed job_semantic_chunks counterpart of IndexOpen's
// single Meili-facing vector (see openspec/changes/drop-hybrid-search-pgvector-similar/
// design.md Decisions 1/1a). ChunkIndex matches job_semantic_chunks.chunk_index. Defined
// here (not reusing internal/search's own JobChunkEmbedding) so this package's ports
// stay independent of internal/search's types — only cmd/embed's concrete adapters know
// about both.
type ChunkEmbedding struct {
	ChunkIndex int16
	Vector     []float32
}

// Store is the persistence the runner needs. The real implementation wraps the
// generated queries and a pool (running CompleteOpen/CompleteClosed in a transaction);
// tests use a fake. Jobs and CompleteOpen trade in db.Job rows — the port hides the
// pool and the transaction boundaries, not the generated row type.
type Store interface {
	// Enqueue adds outbox entries for jobs needing (re-)embedding or removal at model.
	Enqueue(ctx context.Context, targetModel string) (int64, error)
	// Claim leases up to batch live, unleased entries (closed jobs included).
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Jobs returns the persisted rows the documents are built from. A corrupted row
	// aborts the whole load, so the runner retries such a batch per item to isolate it.
	Jobs(ctx context.Context, ids []int64) ([]db.Job, error)
	// CompleteOpen stamps each entry's job embed provenance (model + its current
	// content_hash), persists each job's semantic vector, replaces each job's
	// pgvector-backed chunk rows (job_semantic_chunks), clears its similar_computed_at
	// staleness stamp, and deletes the outbox entries — all atomically. vectors maps
	// job id to the single vector just upserted into the Meili-facing index, so the
	// durable Postgres copy commits with the provenance stamp. chunks maps job id to
	// its ordered chunk vectors (see ChunkEmbedding); a job absent from chunks (an
	// empty/very short description) has its existing chunk rows removed and gets none
	// back — "replace", not "append".
	CompleteOpen(ctx context.Context, entries []Claimed, model string, vectors map[int64][]float32, chunks map[int64][]ChunkEmbedding) error
	// CompleteClosed clears each entry's job embed provenance, deletes each job's
	// pgvector-backed chunk rows (job_semantic_chunks), and deletes the outbox entries,
	// atomically (their documents were just removed from the index).
	CompleteClosed(ctx context.Context, entries []Claimed) error
	// Fail records a failed attempt for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
}

// Indexer is the semantic-index side: embed+upsert open jobs, or remove closed ones.
type Indexer interface {
	// IndexOpen embeds the jobs' documents and upserts their vectors into the semantic
	// index in one batch, returning the vectors keyed by job id so they can be persisted
	// to Postgres alongside the provenance stamp. It ALSO computes each job's chunked
	// embeddings for the pgvector-backed job_semantic_chunks table (design.md Decisions
	// 1/1a) — additive to, and independent of, the single Meili-facing vector: a job
	// with no chunk-worthy description text simply has no entry in the returned chunks
	// map, not an error. Bundled into this SAME call (rather than a separate Indexer
	// method) so processOpenBatch's existing batch-then-per-item-fallback and
	// skip-on-timeout machinery covers the chunk computation too, instead of a second
	// failure-handling path that could drift from the tested one.
	IndexOpen(ctx context.Context, jobs []db.Job) (vectors map[int64][]float32, chunks map[int64][]ChunkEmbedding, err error)
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
	_, err = outbox.RunBatch(ctx, r.Store, outbox.RunOptions{
		BatchSize:    opt.BatchSize,
		LeaseSeconds: opt.LeaseSeconds,
		OnWave: func(outbox.Stats) {
			// A heartbeat per wave so a long drain shows running totals instead of
			// going silent for hours. Reads rn.stats directly (not the generic
			// outbox.Stats parameter) because it distinguishes Indexed from Removed,
			// which outbox's four generic buckets don't carry.
			log.Printf("embed: progress indexed=%d removed=%d failed=%d dead=%d",
				rn.stats.Indexed, rn.stats.Removed, rn.stats.Failed, rn.stats.DeadLettered)
		},
	}, rn.processWave, rn.unreachableFallback)
	if err != nil {
		return rn.stats, fmt.Errorf("claim: %w", err)
	}
	return rn.stats, nil
}

// run accumulates one Run's options and tallies. Waves are processed sequentially (the
// embed concurrency lives inside the Indexer), so the tallies need no lock.
type run struct {
	store   Store
	indexer Indexer
	opt     RunOptions
	stats   Stats
}

// processWave is embed's outbox.BatchProcessor: split the wave into its open and
// closed groups and hand each to its own batch-attempt-then-per-item-fallback cycle
// (unchanged below). Always returns a nil error — embed fully handles every item
// itself, including its own fallback, so outbox.RunBatch's outer per-item Processor
// (unreachableFallback) is never invoked. The returned outbox.Stats is this wave's
// delta, computed from rn.stats before/after, for outbox's own bookkeeping (MaxPerRun,
// OnWave); Run's own public Stats return value comes from rn.stats directly, which
// keeps the Indexed/Removed split outbox.Stats has no room for.
func (rn *run) processWave(ctx context.Context, batch []Claimed) (outbox.Stats, error) {
	var open, closed []Claimed
	for _, e := range batch {
		if e.Closed {
			closed = append(closed, e)
		} else {
			open = append(open, e)
		}
	}
	before := rn.stats
	rn.processOpenBatch(ctx, open)
	rn.processClosedBatch(ctx, closed)
	return outbox.Stats{
		Succeeded:    (rn.stats.Indexed - before.Indexed) + (rn.stats.Removed - before.Removed),
		Failed:       rn.stats.Failed - before.Failed,
		DeadLettered: rn.stats.DeadLettered - before.DeadLettered,
	}, nil
}

// unreachableFallback satisfies outbox.RunBatch's Processor parameter. processWave
// never returns an error, so outbox.RunBatch never calls this — panicking documents
// that invariant rather than silently returning a fabricated outcome.
func (rn *run) unreachableFallback(context.Context, Claimed) outbox.Outcome {
	panic("embed: outer per-item fallback invoked, but processWave never returns an error")
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
	if err != nil {
		if rn.skipOnTimeout(callCtx, entries, "load jobs") {
			return
		}
		rn.fallbackOpen(ctx, entries)
		return
	}
	if len(jobs) != len(entries) {
		rn.fallbackOpen(ctx, entries)
		return
	}
	vectors, chunks, err := rn.indexer.IndexOpen(callCtx, jobs)
	if err != nil {
		if rn.skipOnTimeout(callCtx, entries, "embed/index") {
			return
		}
		rn.fallbackOpen(ctx, entries)
		return
	}
	if err := rn.store.CompleteOpen(callCtx, entries, rn.opt.TargetModel, vectors, chunks); err != nil {
		if rn.skipOnTimeout(callCtx, entries, "complete open") {
			return
		}
		rn.fallbackOpen(ctx, entries)
		return
	}
	rn.stats.Indexed += len(entries)
	log.Printf("embed: indexed batch of %d in %s", len(entries), since(start))
}

// skipOnTimeout reports whether callCtx expired — a normal-but-slow operation simply
// outran CallTimeout, not a per-document defect the semantic index reported — and, if
// so, logs and leaves the wave claimed for its lease to expire, so a later run retries
// the WHOLE batch fresh. Falling back to per-item on a mere timeout would be actively
// harmful here: this index's cost is dominated by a fixed whole-index re-merge (the
// same mechanism internal/searchdrain guards against, see its skipOnTimeout), so a
// single document costs about as much to push as the whole batch, and per-item fallback
// would turn one slow-but-fine batch into up to len(entries) equally slow calls.
func (rn *run) skipOnTimeout(callCtx context.Context, entries []Claimed, stage string) bool {
	if !errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return false
	}
	log.Printf("embed: batch of %d timed out during %s after %s — leaving claimed for "+
		"lease-expiry retry, not falling back to per-item", len(entries), stage, rn.opt.CallTimeout)
	return true
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
		if pgerr.IsDataCorrupted(err) {
			rn.failN(entry, fmt.Errorf("load job: %w", err), 1)
			return
		}
		rn.fail(entry, fmt.Errorf("load job: %w", err))
		return
	}
	if len(jobs) == 0 {
		// A missing row is just as deterministic as a corrupted one — the job was
		// hard-deleted after enqueue, so no future run can load it either. Dead-letter
		// immediately rather than burning the attempt budget across cron runs.
		rn.failN(entry, fmt.Errorf("job %d not found", entry.JobID), 1)
		return
	}
	vectors, chunks, err := rn.indexer.IndexOpen(callCtx, jobs)
	if err != nil {
		rn.fail(entry, fmt.Errorf("embed/index: %w", err))
		return
	}
	if err := rn.store.CompleteOpen(callCtx, []Claimed{entry}, rn.opt.TargetModel, vectors, chunks); err != nil {
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
		if rn.skipOnTimeout(callCtx, entries, "remove closed") {
			return
		}
		rn.fallbackClosed(ctx, entries)
		return
	}
	if err := rn.store.CompleteClosed(callCtx, entries); err != nil {
		if rn.skipOnTimeout(callCtx, entries, "complete closed") {
			return
		}
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
