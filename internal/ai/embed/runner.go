// Package embed drives incremental semantic embedding: enqueue open jobs whose
// chunks are missing or stale (and closed jobs whose embed state must be cleared),
// then drain the semantic_outbox queue wave by wave. Each wave is one TEI batch and
// one Postgres transaction; a batch failure falls back to per-item processing so one
// poison row cannot sink the wave. Mirrors internal/ai/enrich; cmd/embed wires the
// real Store and Indexer adapters.
package embed

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/outbox"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Claimed is one outbox entry leased to this run. Closed means the job is unindexable
// (closed, or a non-canonical repost with duplicate_of set): its embed state is cleared
// instead of embedded.
type Claimed struct {
	OutboxID int64
	JobID    int64
	Closed   bool
}

// ChunkEmbedding is one embedding vector for one 0-indexed chunk of a job's
// description. ChunkIndex matches job_semantic_chunks.chunk_index. Defined here, not
// reusing internal/search's type, so the ports stay independent of it.
type ChunkEmbedding struct {
	ChunkIndex int16
	Vector     []float32
}

// Store is the persistence the runner needs; the real one wraps the generated queries
// and a pool, and runs each Complete* in a transaction.
type Store interface {
	// Enqueue adds outbox entries for jobs needing (re-)embedding or removal at model.
	Enqueue(ctx context.Context, targetModel string) (int64, error)
	// Claim leases up to batch live, unleased entries (closed jobs included).
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Jobs returns the persisted rows. A corrupted row aborts the whole load, so the
	// runner retries that batch per item to isolate it.
	Jobs(ctx context.Context, ids []int64) ([]db.Job, error)
	// CompleteOpen atomically stamps each job's embed provenance (model and current
	// content_hash), replaces its job_semantic_chunks rows and deletes the outbox
	// entries. A job absent from chunks (empty or very short description) has its
	// chunk rows removed and gets none back: replace, not append.
	// inputHashes (Indexer.InputHash per job id) is stamped as
	// jobs.semantic_embedded_input_hash so the next run can tell whether the text moved.
	CompleteOpen(ctx context.Context, entries []Claimed, model string, chunks map[int64][]ChunkEmbedding, inputHashes map[int64]string) error
	// CompleteUnchanged atomically settles entries whose embedded text has not changed
	// (see Indexer.InputHash): re-stamps the provenance so the enqueue predicate stops
	// firing and deletes the outbox entries. Chunk rows are left alone.
	CompleteUnchanged(ctx context.Context, entries []Claimed, model string, inputHashes map[int64]string) error
	// CompleteClosed atomically clears each job's embed provenance, deletes its
	// job_semantic_chunks rows and the outbox entries. That is the whole closed-job
	// side effect.
	CompleteClosed(ctx context.Context, entries []Claimed) error
	// Fail records a failed attempt for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
}

// Indexer is the embedding side: compute each open job's chunked embeddings. There is
// no "remove closed" method; Store.CompleteClosed is the whole closed-job side effect.
type Indexer interface {
	// IndexOpen computes each job's chunked embeddings. A job with no chunk-worthy
	// description has no entry in the returned map; that is not an error.
	IndexOpen(ctx context.Context, jobs []db.Job) (chunks map[int64][]ChunkEmbedding, err error)
	// InputHash fingerprints exactly the fields IndexOpen embeds. The runner compares it
	// with the stored semantic_embedded_input_hash before embedding: content_hash also
	// moves on derived facets the embedder never reads, so it only says "maybe changed".
	InputHash(job db.Job) string
}

// RunOptions are the per-run knobs.
type RunOptions struct {
	// TargetModel is the embedder identity: the enqueue staleness key and the value
	// stamped on a successful embed (search.CurrentEmbedderModel()).
	TargetModel string
	// BatchSize is the claim wave size and the embed/persist batch size. The embed
	// backend chunks internally (EMBED_CONCURRENCY), so it can be large (hundreds).
	BatchSize    int
	LeaseSeconds int
	MaxAttempts  int
	// CallTimeout bounds a single batch's (or fallback item's) embed operation; 0 means
	// no per-call timeout (the embed backend has its own per-attempt timeout).
	CallTimeout time.Duration
	// MaxRun bounds the whole run from its start (enqueue included): no wave is claimed
	// once elapsed time plus CallTimeout would pass it, so the run exits itself instead
	// of being killed mid-wave. 0 means unbounded.
	MaxRun time.Duration
}

// Stats reports what a run did.
type Stats struct {
	Indexed int
	// Unchanged counts open jobs re-stamped without an embed: enqueued because their
	// content_hash moved, but their embedded text had not.
	Unchanged    int
	Removed      int
	Failed       int
	DeadLettered int
}

// Runner drives the process: enqueue outstanding work, then drain claimed waves.
type Runner struct {
	Store   Store
	Indexer Indexer
	// Now is the clock MaxRun is measured on; nil means time.Now.
	Now func() time.Time
}

// Run enqueues outstanding jobs and drains the queue until no claimable entries remain,
// or until opt.MaxRun leaves no room for another wave. A failure on a single entry is
// recorded and never aborts the run.
func (r Runner) Run(ctx context.Context, opt RunOptions) (Stats, error) {
	now := r.Now
	if now == nil {
		now = time.Now
	}
	start := now()
	enqueued, err := r.Store.Enqueue(ctx, opt.TargetModel)
	if err != nil {
		return Stats{}, fmt.Errorf("enqueue: %w", err)
	}
	log.Printf("embed: enqueued %d pending, draining (batch=%d, max run %s)", enqueued, opt.BatchSize, opt.MaxRun)

	rn := &run{store: r.Store, indexer: r.Indexer, opt: opt, now: now, start: start}
	_, err = outbox.RunBatch(ctx, rn, outbox.RunOptions{
		BatchSize:    opt.BatchSize,
		LeaseSeconds: opt.LeaseSeconds,
		OnWave: func(outbox.Stats) {
			// Per-wave heartbeat. Reads rn.stats, not the generic outbox.Stats, which
			// cannot tell Indexed from Removed.
			log.Printf("embed: progress indexed=%d unchanged=%d removed=%d failed=%d dead=%d",
				rn.stats.Indexed, rn.stats.Unchanged, rn.stats.Removed, rn.stats.Failed, rn.stats.DeadLettered)
		},
	}, rn.processWave, nil)
	if err != nil {
		return rn.stats, fmt.Errorf("claim: %w", err)
	}
	return rn.stats, nil
}

// run accumulates one Run's options and tallies. Waves run sequentially (concurrency
// is inside the Indexer), so the tallies need no lock.
type run struct {
	store   Store
	indexer Indexer
	opt     RunOptions
	stats   Stats
	now     func() time.Time
	start   time.Time
}

// Claim is the Store's claim behind the MaxRun budget. Returning no entries is
// outbox.RunBatch's ordinary stop, so an out-of-budget run ends like a drained one.
func (rn *run) Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
	if rn.opt.MaxRun > 0 {
		if elapsed := rn.now().Sub(rn.start); elapsed+rn.opt.CallTimeout > rn.opt.MaxRun {
			log.Printf("embed: stopping after %s — another wave may take up to %s and the run "+
				"budget is %s; the rest stays queued for the next run",
				elapsed.Round(time.Second), rn.opt.CallTimeout, rn.opt.MaxRun)
			return nil, nil
		}
	}
	return rn.store.Claim(ctx, batch, leaseSeconds)
}

// unchanged reports whether job's chunks already embed its current text (hash, its
// Indexer.InputHash) under the target model. A NULL stored hash is never unchanged.
func (rn *run) unchanged(job db.Job, hash string) bool {
	return job.SemanticEmbeddedModel.Valid && job.SemanticEmbeddedModel.String == rn.opt.TargetModel &&
		job.SemanticEmbeddedInputHash.Valid && job.SemanticEmbeddedInputHash.String == hash
}

// processWave is embed's outbox.BatchProcessor: it splits the wave into open and
// closed groups, each with its own batch-then-per-item-fallback cycle. It always
// returns a nil error, so outbox.RunBatch's per-item Processor is never invoked. The
// returned outbox.Stats is this wave's delta; Run's public Stats come from rn.stats.
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
		Succeeded: (rn.stats.Indexed - before.Indexed) + (rn.stats.Unchanged - before.Unchanged) +
			(rn.stats.Removed - before.Removed),
		Failed:       rn.stats.Failed - before.Failed,
		DeadLettered: rn.stats.DeadLettered - before.DeadLettered,
	}, nil
}

// processOpenBatch embeds a whole wave of open jobs in one batch and completes them in
// one transaction. Any batch-level failure (corrupted-row load, embed error, partial
// load) falls back to per-item processing.
func (rn *run) processOpenBatch(ctx context.Context, entries []Claimed) {
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
		rn.fallbackOpen(ctx, entries)
		return
	}
	if len(jobs) != len(entries) {
		rn.fallbackOpen(ctx, entries)
		return
	}

	// Jobs whose text is unchanged only need their stamp refreshed; the rest are embedded.
	entryOf := make(map[int64]Claimed, len(entries))
	for _, e := range entries {
		entryOf[e.JobID] = e
	}
	hashes := make(map[int64]string, len(jobs))
	var same, changed []Claimed
	var changedJobs []db.Job
	for _, j := range jobs {
		hashes[j.ID] = rn.indexer.InputHash(j)
		if rn.unchanged(j, hashes[j.ID]) {
			same = append(same, entryOf[j.ID])
		} else {
			changed = append(changed, entryOf[j.ID])
			changedJobs = append(changedJobs, j)
		}
	}
	if len(same) > 0 {
		if err := rn.store.CompleteUnchanged(callCtx, same, rn.opt.TargetModel, hashes); err != nil {
			if rn.skipOnContextEnd(callCtx, entries, "complete unchanged") {
				return
			}
			rn.fallbackOpen(ctx, entries)
			return
		}
		rn.stats.Unchanged += len(same)
	}
	if len(changed) == 0 {
		log.Printf("embed: re-stamped %d unchanged in %s", len(same), since(start))
		return
	}

	chunks, err := rn.indexer.IndexOpen(callCtx, changedJobs)
	if err != nil {
		if rn.skipOnContextEnd(callCtx, changed, "embed/index") {
			return
		}
		rn.fallbackOpen(ctx, changed)
		return
	}
	if err := rn.store.CompleteOpen(callCtx, changed, rn.opt.TargetModel, chunks, hashes); err != nil {
		if rn.skipOnContextEnd(callCtx, changed, "complete open") {
			return
		}
		rn.fallbackOpen(ctx, changed)
		return
	}
	rn.stats.Indexed += len(changed)
	log.Printf("embed: indexed batch of %d (+%d unchanged, re-stamped) in %s", len(changed), len(same), since(start))
}

// skipOnContextEnd reports whether callCtx ended before the stage finished (CallTimeout
// outrun, or the run cancelled) and, if so, logs and leaves the wave claimed so a later
// run retries the WHOLE batch after the lease expires. It checks the context, not the
// error, so it does not depend on how the backend or driver wrapped it.
//
// Per-item fallback on a timeout would multiply TEI calls and Postgres transactions by
// len(entries). On a cancellation it is worse: Fail writes on a background context, so a
// redeploy's SIGTERM would charge the wave an attempt, and three dead-letter the entry
// permanently (nothing re-enqueues it) and /similar stays empty for that posting.
func (rn *run) skipOnContextEnd(callCtx context.Context, entries []Claimed, stage string) bool {
	cause := callCtx.Err()
	if cause == nil {
		return false
	}
	log.Printf("embed: batch of %d abandoned during %s (%v, budget %s) — leaving claimed for "+
		"lease-expiry retry, not falling back to per-item", len(entries), stage, cause, rn.opt.CallTimeout)
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
		// A corrupted row (XX001) can never load: dead-letter it now (mirrors enrich).
		if pgerr.IsDataCorrupted(err) {
			rn.failN(entry, fmt.Errorf("load job: %w", err), 1)
			return
		}
		rn.fail(callCtx, entry, fmt.Errorf("load job: %w", err))
		return
	}
	if len(jobs) == 0 {
		// Hard-deleted after enqueue: as unloadable as a corrupted row, dead-letter now.
		rn.failN(entry, fmt.Errorf("job %d not found", entry.JobID), 1)
		return
	}
	hashes := map[int64]string{entry.JobID: rn.indexer.InputHash(jobs[0])}
	if rn.unchanged(jobs[0], hashes[entry.JobID]) {
		if err := rn.store.CompleteUnchanged(callCtx, []Claimed{entry}, rn.opt.TargetModel, hashes); err != nil {
			rn.fail(callCtx, entry, fmt.Errorf("complete unchanged: %w", err))
			return
		}
		rn.stats.Unchanged++
		return
	}
	chunks, err := rn.indexer.IndexOpen(callCtx, jobs)
	if err != nil {
		rn.fail(callCtx, entry, fmt.Errorf("embed/index: %w", err))
		return
	}
	if err := rn.store.CompleteOpen(callCtx, []Claimed{entry}, rn.opt.TargetModel, chunks, hashes); err != nil {
		rn.fail(callCtx, entry, fmt.Errorf("complete open: %w", err))
		return
	}
	rn.stats.Indexed++
}

// processClosedBatch clears a whole wave of closed jobs' embed state in one batch,
// with the same per-item fallback on failure.
func (rn *run) processClosedBatch(ctx context.Context, entries []Claimed) {
	if len(entries) == 0 {
		return
	}
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	if err := rn.store.CompleteClosed(callCtx, entries); err != nil {
		if rn.skipOnContextEnd(callCtx, entries, "complete closed") {
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

	if err := rn.store.CompleteClosed(callCtx, []Claimed{entry}); err != nil {
		rn.fail(callCtx, entry, fmt.Errorf("complete closed: %w", err))
		return
	}
	rn.stats.Removed++
}

// callContext derives the per-batch timeout context (no timeout when CallTimeout is 0).
func (rn *run) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if rn.opt.CallTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, rn.opt.CallTimeout)
}

// fail records a failed attempt against one entry, unless the call context ended first.
// A wave abandoned mid-flight (CallTimeout, SIGTERM) must not be charged an attempt:
// failN writes on a background context, and three attempts dead-letter the entry for good.
func (rn *run) fail(callCtx context.Context, entry Claimed, cause error) {
	if ctxErr := callCtx.Err(); ctxErr != nil {
		log.Printf("embed: outbox=%d abandoned (%v), leaving claimed for lease-expiry retry: %v",
			entry.OutboxID, ctxErr, cause)
		return
	}
	rn.failN(entry, cause, rn.opt.MaxAttempts)
}

// failN records a failure with an explicit attempt ceiling; 1 dead-letters immediately.
func (rn *run) failN(entry Claimed, cause error, maxAttempts int) {
	// Background context: a timed-out or cancelled call must still record its failure.
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
