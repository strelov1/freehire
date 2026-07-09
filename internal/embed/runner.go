// Package embed drives incremental semantic embedding: enqueue open jobs whose vector
// is missing/stale (and closed jobs whose vector must be removed), then drain the
// semantic_outbox queue wave by wave, embedding+upserting open jobs and removing closed
// ones in place. It mirrors internal/enrich: the Runner is independent of the DB and
// search layers (Store + Indexer ports), so the branch/fail logic is unit-tested with
// fakes; cmd/embed wires the real adapters.
package embed

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

// Claimed is one outbox entry leased to this run. Closed marks whether the job has
// since closed: open jobs are embedded, closed jobs have their document removed.
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
	// Job returns the persisted row a document is built from.
	Job(ctx context.Context, id int64) (db.Job, error)
	// CompleteOpen stamps the job's embed provenance (model + the exact embedded
	// content hash) and deletes the outbox entry, atomically.
	CompleteOpen(ctx context.Context, entry Claimed, model string, hash pgtype.Text) error
	// CompleteClosed clears the job's embed provenance and deletes the outbox entry,
	// atomically (its document was just removed from the index).
	CompleteClosed(ctx context.Context, entry Claimed) error
	// Fail records a failed attempt; it returns whether the entry was dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
}

// Indexer is the semantic-index side: embed+upsert an open job, or remove a closed one.
type Indexer interface {
	// IndexOpen embeds the job's document and upserts its vector into the semantic index.
	IndexOpen(ctx context.Context, job db.Job) error
	// RemoveClosed deletes the job's document from the semantic index.
	RemoveClosed(ctx context.Context, jobID int64) error
}

// RunOptions are the per-run knobs.
type RunOptions struct {
	// TargetModel is the embedder identity: the enqueue staleness key and the value
	// stamped on a successful embed (search.CurrentEmbedderModel()).
	TargetModel string
	// Concurrency is both the number of embeds in flight and the claim wave size, so
	// each claimed entry's lease window stays ≈ one embed call.
	Concurrency  int
	LeaseSeconds int
	MaxAttempts  int
	// CallTimeout bounds a single job's index/remove operation; 0 means no per-call
	// timeout (the embed backend has its own per-attempt timeout regardless).
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
	log.Printf("embed: enqueued %d pending, draining (concurrency=%d)", enqueued, opt.Concurrency)

	rn := &run{store: r.Store, indexer: r.Indexer, opt: opt}
	for {
		// A wave the size of the concurrency, drained in parallel so each entry's lease
		// window stays ≈ one embed call.
		batch, err := r.Store.Claim(ctx, opt.Concurrency, opt.LeaseSeconds)
		if err != nil {
			return rn.stats, fmt.Errorf("claim: %w", err)
		}
		if len(batch) == 0 {
			return rn.stats, nil
		}
		var wg sync.WaitGroup
		for _, entry := range batch {
			wg.Add(1)
			go func(e Claimed) {
				defer wg.Done()
				rn.process(ctx, e)
			}(entry)
		}
		wg.Wait()
		log.Printf("embed: progress indexed=%d removed=%d failed=%d dead=%d",
			rn.stats.Indexed, rn.stats.Removed, rn.stats.Failed, rn.stats.DeadLettered)
	}
}

// run accumulates one Run's options and tallies; a wave's workers process entries
// concurrently, so the tallies are guarded by mu.
type run struct {
	store   Store
	indexer Indexer
	opt     RunOptions

	mu    sync.Mutex
	stats Stats
}

// process handles one claimed entry, branching on whether the job is open (embed) or
// closed (remove). Any failure routes to fail so the run continues with the rest.
func (rn *run) process(ctx context.Context, entry Claimed) {
	start := time.Now()
	callCtx, cancel := rn.callContext(ctx)
	defer cancel()

	if entry.Closed {
		rn.processClosed(callCtx, entry, start)
		return
	}
	rn.processOpen(callCtx, entry, start)
}

func (rn *run) processOpen(ctx context.Context, entry Claimed, start time.Time) {
	job, err := rn.store.Job(ctx, entry.JobID)
	if err != nil {
		// A corrupted row (XX001) can never load — dead-letter it immediately rather
		// than burning the attempt budget across cron runs (mirrors enrich).
		if worker.IsCorruptedRow(err) {
			rn.failN(entry, fmt.Errorf("load job: %w", err), 1)
			log.Printf("embed: job=%d dead-lettered (corrupted row) in %s: %v", entry.JobID, since(start), err)
			return
		}
		rn.fail(entry, fmt.Errorf("load job: %w", err))
		log.Printf("embed: job=%d load failed in %s: %v", entry.JobID, since(start), err)
		return
	}

	if err := rn.indexer.IndexOpen(ctx, job); err != nil {
		rn.fail(entry, fmt.Errorf("embed/index: %w", err))
		log.Printf("embed: job=%d index FAILED in %s: %v", entry.JobID, since(start), err)
		return
	}
	if err := rn.store.CompleteOpen(ctx, entry, rn.opt.TargetModel, job.ContentHash); err != nil {
		rn.fail(entry, fmt.Errorf("complete open: %w", err))
		log.Printf("embed: job=%d complete failed in %s: %v", entry.JobID, since(start), err)
		return
	}
	rn.tally(func(s *Stats) { s.Indexed++ })
	log.Printf("embed: job=%d indexed in %s", entry.JobID, since(start))
}

func (rn *run) processClosed(ctx context.Context, entry Claimed, start time.Time) {
	if err := rn.indexer.RemoveClosed(ctx, entry.JobID); err != nil {
		rn.fail(entry, fmt.Errorf("remove closed: %w", err))
		log.Printf("embed: job=%d remove FAILED in %s: %v", entry.JobID, since(start), err)
		return
	}
	if err := rn.store.CompleteClosed(ctx, entry); err != nil {
		rn.fail(entry, fmt.Errorf("complete closed: %w", err))
		log.Printf("embed: job=%d complete-closed failed in %s: %v", entry.JobID, since(start), err)
		return
	}
	rn.tally(func(s *Stats) { s.Removed++ })
	log.Printf("embed: job=%d removed (closed) in %s", entry.JobID, since(start))
}

// callContext derives the per-entry timeout context (no-op when CallTimeout is 0).
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
	rn.tally(func(s *Stats) {
		if err == nil && dead {
			s.DeadLettered++
			return
		}
		s.Failed++
	})
}

func (rn *run) tally(f func(*Stats)) {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	f(&rn.stats)
}

func since(t time.Time) time.Duration { return time.Since(t).Round(time.Millisecond) }
