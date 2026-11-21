package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/platform/outbox"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Claimed is one outbox entry leased to this run.
type Claimed struct {
	OutboxID      int64
	JobID         int64
	TargetVersion int
}

// Store is the persistence the runner needs, in domain terms so the runner is independent
// of the DB layer.
type Store interface {
	// Enqueue adds outbox entries for jobs not yet enriched to targetVersion. It runs
	// concurrently with Claim and the rest, and may take longer than a whole run.
	Enqueue(ctx context.Context, targetVersion int) (int64, error)
	// Claim leases up to batch live, unleased entries.
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Job returns the source fields a Provider reads for the given job id.
	Job(ctx context.Context, id int64) (JobInput, error)
	// Complete writes the enrichment payload + provenance stamp to the entry's job
	// and deletes the outbox entry, atomically.
	Complete(ctx context.Context, entry Claimed, payload json.RawMessage) error
	// Fail records a failed attempt under the given policy; it returns whether the
	// entry was dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, policy FailurePolicy) (deadLettered bool, err error)
	// Reap deletes up to maxRows live entries Claim can never take (their job closed,
	// became a non-canonical repost, or is gone), returning how many it removed.
	Reap(ctx context.Context, maxRows int) (int64, error)
}

// FailurePolicy is how one failure is bounded, decided by PostingAtFault. MaxAttempts counts
// tries at a possibly impossible task; UpstreamGraceDays waits out a dependency expected
// back. See postingAtFault and RecordEnrichmentFailure.
type FailurePolicy struct {
	PostingAtFault    bool
	MaxAttempts       int
	UpstreamGraceDays int
}

// RunOptions are the per-run knobs.
type RunOptions struct {
	TargetVersion int
	// Concurrency is both the LLM calls in flight and the claim wave size, which keeps each
	// entry's lease window about one call so an overlapping run can't reclaim it.
	Concurrency  int
	LeaseSeconds int
	// MaxAttempts bounds failures the posting caused; UpstreamGraceDays bounds every
	// other failure, by the entry's queue age rather than by tries.
	MaxAttempts       int
	UpstreamGraceDays int
}

// Stats reports what a run did.
type Stats struct {
	Enriched     int
	Failed       int
	DeadLettered int
	// Reaped counts entries deleted because Claim could never take them.
	Reaped int
}

// reapLimit bounds how many ineligible entries one run deletes: an unbounded pass over a
// large backlog holds row locks the claim then waits on, so each run takes a slice.
const reapLimit = 20_000

// Progress counts enrichments this process has written, readable mid-run (Stats exists only
// after Run returns). Safe for concurrent use.
type Progress struct {
	completed atomic.Int64
}

// Completed reports how many enrichments have been written so far.
func (p *Progress) Completed() int64 { return p.completed.Load() }

// Runner enqueues pending jobs, then drains claimed batches, enriching and writing back
// each entry.
type Runner struct {
	Provider Provider
	Store    Store
	// Progress, when set, advances once per written enrichment.
	Progress *Progress
	// wait pauses between waves while upstream refuses every call; nil sleeps for real.
	// Tests replace it.
	wait func(context.Context, time.Duration)
}

// The pause between waves while the upstream refuses every call starts at backoffMin,
// doubles per refused wave and stops at backoffMax. Without it a spent provider quota meant
// hundreds of refused calls a minute, burning a key pool other consumers share; the cap
// keeps recovery prompt.
const (
	backoffMin = 30 * time.Second
	backoffMax = 15 * time.Minute
)

// upstreamBackoff decides the pause after each wave. A wave that wrote nothing and had an
// upstream failure grows the pause; any write or no upstream failure resets it. Posting
// faults never pause the drain.
type upstreamBackoff struct {
	cur                time.Duration
	lastSucceeded      int
	lastUpstreamFailed int64
}

// next returns the pause before the next wave from the run's cumulative counts.
func (b *upstreamBackoff) next(succeeded int, upstreamFailed int64) time.Duration {
	wrote := succeeded > b.lastSucceeded
	refused := upstreamFailed > b.lastUpstreamFailed
	b.lastSucceeded, b.lastUpstreamFailed = succeeded, upstreamFailed
	if wrote || !refused {
		b.cur = 0
		return 0
	}
	b.cur = min(max(b.cur*2, backoffMin), backoffMax)
	return b.cur
}

// sleepCtx waits d, or less if ctx ends first.
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Run enqueues pending jobs and drains the queue until no claimable entries remain.
// A failure on a single entry is recorded and never aborts the run.
//
// The enqueue runs alongside the drain, not before it: a whole-catalogue pass takes longer
// than the gap between deploys, and every deploy restarts the run, so a drain that waited
// for it never started. While the enqueue runs, an idle drain retries each enqueuePoll.
func (r Runner) Run(ctx context.Context, opt RunOptions) (Stats, error) {
	// Reap first so the reported depth is work, not residue. Best-effort: housekeeping must
	// never stop enrichment.
	reaped, err := r.Store.Reap(ctx, reapLimit)
	if err != nil {
		log.Printf("enrich: reap ineligible entries (continuing): %v", err)
	} else if reaped > 0 {
		log.Printf("enrich: reaped %d entries whose job closed or became a repost", reaped)
	}

	enqCtx, stopEnqueue := context.WithCancel(ctx)
	defer stopEnqueue()
	type enqueueResult struct {
		n   int64
		err error
	}
	enq := make(chan enqueueResult, 1)
	go func() {
		n, err := r.Store.Enqueue(enqCtx, opt.TargetVersion)
		enq <- enqueueResult{n, err}
	}()
	log.Printf("enrich: draining (concurrency=%d) while pending jobs are enqueued", opt.Concurrency)

	rn := &run{provider: r.Provider, store: r.Store, opt: opt, progress: r.Progress}
	wait := r.wait
	if wait == nil {
		wait = sleepCtx
	}
	stats := Stats{Reaped: int(reaped)}
	// One backoff for the whole run, fed run-wide counts, so an outage keeps growing the
	// pause across the drain passes below.
	var backoff upstreamBackoff
	drain := func() error {
		s, err := outbox.RunPool(ctx, r.Store, outbox.RunOptions{
			// Wave sized to the concurrency so each entry's lease stays about one LLM call.
			BatchSize:    opt.Concurrency,
			LeaseSeconds: opt.LeaseSeconds,
			Concurrency:  opt.Concurrency,
			OnWave: func(cum outbox.Stats) {
				// Heartbeat so a long drain isn't silent.
				log.Printf("enrich: progress enriched=%d failed=%d dead=%d", stats.Enriched+cum.Succeeded, stats.Failed+cum.Failed, stats.DeadLettered+cum.DeadLettered)
				// OnWave runs between waves with nothing claimed, so pausing here holds no lease.
				if d := backoff.next(stats.Enriched+cum.Succeeded, rn.upstreamFailed.Load()); d > 0 {
					log.Printf("enrich: whole wave refused upstream, pausing %s", d)
					wait(ctx, d)
				}
			},
		}, rn.process)
		stats.Enriched += s.Succeeded
		stats.Failed += s.Failed
		stats.DeadLettered += s.DeadLettered
		if err != nil {
			return fmt.Errorf("claim: %w", err)
		}
		return nil
	}

	// Drain, and while the enqueue is still going, drain again each enqueuePoll so the
	// chunks it commits are taken as they land; stop after a drain that follows its end.
	enqueueDone := false
	for {
		if err := drain(); err != nil {
			stopEnqueue()
			if !enqueueDone {
				<-enq
			}
			return stats, err
		}
		if enqueueDone {
			return stats, nil
		}
		select {
		case res := <-enq:
			enqueueDone = true
			if res.err != nil {
				return stats, fmt.Errorf("enqueue: %w", res.err)
			}
			log.Printf("enrich: enqueued %d pending", res.n)
		case <-time.After(enqueuePoll):
		case <-ctx.Done():
			<-enq
			return stats, ctx.Err()
		}
	}
}

// enqueuePoll is how often an idle drain looks again for work while the enqueue runs.
// A var so tests can shorten it.
var enqueuePoll = time.Minute

// run carries one Run's dependencies so the per-entry helpers use the receiver.
type run struct {
	provider Provider
	store    Store
	opt      RunOptions
	progress *Progress
	// upstreamFailed counts failures the posting did not cause; the backoff reads it.
	upstreamFailed atomic.Int64
}

// process handles one claimed entry; outbox.RunPool tallies the outcome.
func (rn *run) process(ctx context.Context, entry Claimed) outbox.Outcome {
	start := time.Now()

	job, err := rn.store.Job(ctx, entry.JobID)
	if err != nil {
		// A corrupted row (XX001) can never load, so dead-letter it immediately.
		if pgerr.IsDataCorrupted(err) {
			outcome := rn.failN(ctx, entry, fmt.Errorf("load job: %w", err), 1)
			log.Printf("enrich: job=%d dead-lettered (corrupted row) in %s: %v", entry.JobID, time.Since(start).Round(time.Millisecond), err)
			return outcome
		}
		outcome := rn.fail(ctx, entry, fmt.Errorf("load job: %w", err))
		log.Printf("enrich: job=%d load failed in %s: %v", entry.JobID, time.Since(start).Round(time.Millisecond), err)
		return outcome
	}

	enr, err := rn.enrich(ctx, job)
	if err != nil {
		outcome := rn.fail(ctx, entry, err)
		log.Printf("enrich: job=%d FAILED in %s: %v", entry.JobID, time.Since(start).Round(time.Millisecond), err)
		return outcome
	}

	payload, err := json.Marshal(enr)
	if err != nil {
		outcome := rn.fail(ctx, entry, fmt.Errorf("marshal: %w", err))
		log.Printf("enrich: job=%d marshal failed: %v", entry.JobID, err)
		return outcome
	}

	if err := rn.store.Complete(ctx, entry, payload); err != nil {
		outcome := rn.fail(ctx, entry, fmt.Errorf("write back: %w", err))
		log.Printf("enrich: job=%d write-back failed: %v", entry.JobID, err)
		return outcome
	}
	if rn.progress != nil {
		rn.progress.completed.Add(1)
	}
	log.Printf("enrich: job=%d ok in %s", entry.JobID, time.Since(start).Round(time.Millisecond))
	return outbox.Succeeded
}

// enrich asks the provider for a payload and validates it, retrying once on a transient
// transport failure. Deterministic failures (unparseable response, payload invalid after
// sanitizing) are not retried in-process; the outbox re-tries them next cron run.
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
		// Drop out-of-vocabulary values rather than fail the payload over one stray field.
		enr.Sanitize()
		if err := enr.Validate(); err != nil {
			// A failure after Sanitize is structural. Wrapped so postingAtFault recognises
			// it; Validate stays a plain error since the gate has other callers.
			return Enrichment{}, fmt.Errorf("%w: %w", errInvalidPayload, err)
		}
		return enr, nil
	}
	return Enrichment{}, lastErr
}

func (rn *run) fail(ctx context.Context, entry Claimed, cause error) outbox.Outcome {
	return rn.failN(ctx, entry, cause, rn.opt.MaxAttempts)
}

// failN records a failure with an explicit attempt ceiling; the corrupted-row path passes 1
// to dead-letter at once. The ceiling applies only when postingAtFault says the posting is
// at fault; everything else is bounded by queue age, so an outage costs nothing permanently.
func (rn *run) failN(ctx context.Context, entry Claimed, cause error, maxAttempts int) outbox.Outcome {
	policy := FailurePolicy{
		PostingAtFault:    postingAtFault(cause),
		MaxAttempts:       maxAttempts,
		UpstreamGraceDays: rn.opt.UpstreamGraceDays,
	}
	if !policy.PostingAtFault {
		rn.upstreamFailed.Add(1)
	}
	dead, err := rn.store.Fail(ctx, entry.OutboxID, cause.Error(), policy)
	if err != nil {
		// Log the cause so a bookkeeping outage doesn't hide behind a bare Failed tally.
		log.Printf("enrich: outbox=%d fail-bookkeeping error: %v", entry.OutboxID, err)
	}
	// A Fail that couldn't be recorded counts as a plain failure.
	if err == nil && dead {
		return outbox.DeadLettered
	}
	return outbox.Failed
}
