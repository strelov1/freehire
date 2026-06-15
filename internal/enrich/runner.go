package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// Claimed is one outbox entry leased to this run.
type Claimed struct {
	OutboxID      int64
	JobID         int64
	TargetVersion int
}

// Store is the persistence the runner needs, expressed in domain terms so the
// runner is independent of the DB layer. The real implementation wraps the
// generated queries and a connection pool (and runs Complete in a transaction);
// tests use an in-memory fake.
type Store interface {
	// Enqueue adds outbox entries for jobs not yet enriched to targetVersion.
	Enqueue(ctx context.Context, targetVersion int) (int64, error)
	// Claim leases up to batch live, unleased entries.
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Job returns the source fields a Provider reads for the given job id.
	Job(ctx context.Context, id int64) (JobInput, error)
	// Complete writes the enrichment payload + provenance stamp to the entry's job
	// and deletes the outbox entry, atomically.
	Complete(ctx context.Context, entry Claimed, payload json.RawMessage) error
	// Fail records a failed attempt; it returns whether the entry was dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
}

// RunOptions are the per-run knobs.
type RunOptions struct {
	TargetVersion int
	// Concurrency is both the number of LLM calls in flight and the claim wave size.
	// Sizing the wave to the concurrency keeps each claimed entry's lease window short
	// (≈ one LLM call), so an overlapping run can't reclaim a still-in-flight entry.
	Concurrency  int
	LeaseSeconds int
	MaxAttempts  int
}

// Stats reports what a run did.
type Stats struct {
	Enriched     int
	Failed       int
	DeadLettered int
}

// Runner drives the enrichment process: enqueue pending jobs, then drain claimed
// batches, enriching and writing back each entry.
type Runner struct {
	Provider Provider
	Store    Store
}

// Run enqueues pending jobs and drains the queue until no claimable entries remain.
// A failure on a single entry is recorded and never aborts the run.
func (r Runner) Run(ctx context.Context, opt RunOptions) (Stats, error) {
	enqueued, err := r.Store.Enqueue(ctx, opt.TargetVersion)
	if err != nil {
		return Stats{}, fmt.Errorf("enqueue: %w", err)
	}
	log.Printf("enrich: enqueued %d pending, draining (concurrency=%d)", enqueued, opt.Concurrency)

	rn := &run{provider: r.Provider, store: r.Store, opt: opt}
	for {
		// Claim a wave the size of the concurrency, then drain it in parallel: each
		// entry starts processing at once, so its lease window stays ≈ one LLM call.
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
		// A heartbeat per wave so a long drain shows running totals instead of
		// going silent for hours.
		log.Printf("enrich: progress enriched=%d failed=%d dead=%d", rn.stats.Enriched, rn.stats.Failed, rn.stats.DeadLettered)
	}
}

// run accumulates one Run's options and tallies so the per-entry helpers carry the
// receiver instead of threading opt and a *Stats through every call. A wave's workers
// process entries concurrently, so the tallies are guarded by mu.
type run struct {
	provider Provider
	store    Store
	opt      RunOptions

	mu    sync.Mutex
	stats Stats
}

// process handles one claimed entry. Any failure routes to fail so the run
// continues with the remaining entries. Each entry logs its outcome and duration
// so a long drain is observable in real time.
func (rn *run) process(ctx context.Context, entry Claimed) {
	start := time.Now()

	job, err := rn.store.Job(ctx, entry.JobID)
	if err != nil {
		rn.fail(ctx, entry, fmt.Errorf("load job: %w", err))
		log.Printf("enrich: job=%d load failed in %s: %v", entry.JobID, time.Since(start).Round(time.Millisecond), err)
		return
	}

	enr, err := rn.enrich(ctx, job)
	if err != nil {
		rn.fail(ctx, entry, err)
		log.Printf("enrich: job=%d FAILED in %s: %v", entry.JobID, time.Since(start).Round(time.Millisecond), err)
		return
	}

	payload, err := json.Marshal(enr)
	if err != nil {
		rn.fail(ctx, entry, fmt.Errorf("marshal: %w", err))
		log.Printf("enrich: job=%d marshal failed: %v", entry.JobID, err)
		return
	}

	if err := rn.store.Complete(ctx, entry, payload); err != nil {
		rn.fail(ctx, entry, fmt.Errorf("write back: %w", err))
		log.Printf("enrich: job=%d write-back failed: %v", entry.JobID, err)
		return
	}
	rn.mu.Lock()
	rn.stats.Enriched++
	rn.mu.Unlock()
	log.Printf("enrich: job=%d ok in %s", entry.JobID, time.Since(start).Round(time.Millisecond))
}

// enrich asks the provider for a payload and validates it, retrying once on a
// transient transport failure. An invalid payload is treated as an error so it
// is never persisted. Deterministic failures (an unparseable response, or a
// payload that fails validation even after sanitizing) are not retried
// in-process — an identical re-call would reproduce them; the outbox attempts
// counter re-tries them on the next cron run, drawing a fresh sample.
func (rn *run) enrich(ctx context.Context, job JobInput) (Enrichment, error) {
	var lastErr error
	for range 2 {
		enr, err := rn.provider.Enrich(ctx, job)
		if err != nil {
			lastErr = err
			if errors.Is(err, errUnparseableResponse) {
				break
			}
			continue
		}
		// Drop any out-of-vocabulary enum values rather than failing the whole
		// payload over one stray field; Validate is then a guard that should pass.
		enr.Sanitize()
		if err := enr.Validate(); err != nil {
			// Sanitize already dropped out-of-vocab values, so a Validate failure
			// is a structural problem a retry won't fix.
			return Enrichment{}, err
		}
		return enr, nil
	}
	return Enrichment{}, lastErr
}

func (rn *run) fail(ctx context.Context, entry Claimed, cause error) {
	dead, err := rn.store.Fail(ctx, entry.OutboxID, cause.Error(), rn.opt.MaxAttempts)
	if err != nil {
		// The attempt still counts as a failure below, but log the cause: a
		// bookkeeping outage (e.g. the DB going unreachable mid-drain) would
		// otherwise hide behind an opaque Failed tally with no way to diagnose it.
		log.Printf("enrich: outbox=%d fail-bookkeeping error: %v", entry.OutboxID, err)
	}
	rn.mu.Lock()
	defer rn.mu.Unlock()
	// Only a recorded dead-letter is distinct; a non-dead attempt and a Fail that
	// couldn't even be recorded both count as a plain failure.
	if err == nil && dead {
		rn.stats.DeadLettered++
		return
	}
	rn.stats.Failed++
}
