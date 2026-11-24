package applyform

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/platform/outbox"
)

// Claimed is one capture leased to this run. Provider and ExternalID come straight off the
// posting's row, which is everything a fetch needs — the external id is the board and the
// platform's posting id joined by externalid.Namespace.
type Claimed struct {
	OutboxID   int64
	JobID      int64
	Provider   string
	ExternalID string
	// URL is the posting's own address. Most fetchers ignore it — the board and posting
	// id are enough — but a platform with more than one regional host cannot be reached
	// without it, and the host cannot be discovered by trying: Lever answers 404 from the
	// wrong region, which is the same code it uses for a posting that no longer exists.
	URL string
}

// Store is the persistence the runner needs. The real implementation wraps the generated
// queries and a pool; tests use a fake. The port hides the pool and the transaction
// boundaries, not the queue's semantics.
type Store interface {
	// Claim leases up to batch live, unleased captures, none of them from a provider
	// named in skip.
	Claim(ctx context.Context, batch, leaseSeconds int, skip []string) ([]Claimed, error)
	// Save stores the captured form and retires the queue entry, atomically — so a
	// capture cannot be both stored and left queued, nor retired without being stored.
	Save(ctx context.Context, outboxID, jobID int64, form Form) error
	// Fail records a failed attempt for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
	// Discard retires an entry whose posting the platform no longer has, storing nothing.
	Discard(ctx context.Context, outboxID int64) error
}

// RunOptions are the per-run knobs.
type RunOptions struct {
	// BatchSize is how many captures one claim wave leases.
	BatchSize int
	// LeaseSeconds is how long a claim is held before another run may take it — the
	// crash reaper, since a worker that dies mid-capture never releases its entries.
	LeaseSeconds int
	// MaxAttempts bounds the retries before a capture is dead-lettered.
	MaxAttempts int
	// Concurrency bounds how many postings are fetched at once. It is the only thing
	// stopping a backlog drain from becoming a flood aimed at one platform.
	Concurrency int
	// MaxPerRun bounds how much of the backlog ONE run takes; 0 is unbounded.
	//
	// Concurrency limits how fast, this limits how long, and the second bound is the one
	// that matters operationally. Nothing here holds a lock — what keeps a cron worker
	// from stacking on itself is systemd refusing to start a second instance of a
	// Type=oneshot unit while the first is active. So a run that works through a 185k
	// backlog for hours does not merely take a long time: it silently swallows every
	// scheduled firing behind it while ingest keeps enqueueing. Bounded, the cron cadence
	// is what sets throughput, which is where that decision belongs.
	MaxPerRun int
	// CallTimeout bounds a single fetch; 0 leaves it to the transport's own timeout.
	CallTimeout time.Duration
}

// RunStats is what the run did. DeadLettered is counted apart from Failed because a
// failure is a retry and a dead letter is work abandoned — a queue quietly filling with
// the latter is not a healthy queue.
//
// Before this package's migration onto internal/platform/outbox, the implementation
// contradicted that documented intent: a dead-lettered capture incremented BOTH
// Failed and DeadLettered (a pre-existing double-count this doc comment's "apart
// from" never described). internal/platform/outbox.Outcome is structurally one-outcome-per-
// item, so migrating onto it corrects Failed/DeadLettered to the mutually exclusive
// counters already documented here — matching how enrich/embed/search-drain already
// counted them. Found and fixed by code review, not the original design.
type RunStats struct {
	Captured int
	Failed   int
	// Gone counts postings the platform no longer has. They are retired from the queue
	// rather than retried, and counted apart from Failed because they are ordinary
	// attrition — an employer took the posting down and our sweep has not caught up yet.
	Gone         int
	DeadLettered int
	// Deferred counts captures put back untouched because their platform answered 429
	// during this run. They are not failures: no attempt is charged, and the next run
	// tries them again.
	Deferred int
}

// Degraded reports whether a run deserves an alert, and it deliberately does NOT use the
// shared worker.ExitCode rule that every other worker here applies.
//
// That rule — non-zero on any per-item failure — fits a worker whose items rarely fail. This
// one issues hundreds of thousands of requests to other companies' APIs, where a handful of
// transient failures per run is the normal, healthy shape: the queue retries them, and the
// first production run measured 10 in 2490. Marking that degraded paints the unit red on
// nearly every firing, and a status that is always red carries no information.
//
// What does deserve an alert is work ABANDONED rather than postponed: a dead letter is a
// capture nobody will look at again. And a run that captured nothing at all while failing is
// not a blip either — that is the platform being gone, and it must not hide behind the same
// forgiveness the blips get.
func (s RunStats) Degraded() bool {
	return s.DeadLettered > 0 || (s.Captured == 0 && s.Failed > 0)
}

// Run drains the capture queue and returns. It is the whole worker: claim a wave, fetch
// each posting's form through its provider's fetcher, store it, and keep going until a
// wave comes back empty.
//
// It returns an error only when the QUEUE itself is unusable — there is no per-item
// recovery from being unable to claim. Every other failure belongs to one capture and is
// recorded against it, so one platform having a bad afternoon costs its own postings and
// nothing else.
func Run(ctx context.Context, s Store, fetchers map[string]Fetcher, opts RunOptions) (RunStats, error) {
	rn := &run{store: s, fetchers: fetchers, opts: opts, throttled: &throttle{}}
	result, err := outbox.RunPool(ctx, &outbox.StopOnCancel[Claimed]{Claimer: outbox.ClaimFunc[Claimed](
		func(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
			return s.Claim(ctx, batch, leaseSeconds, rn.throttled.list())
		})}, outbox.RunOptions{
		BatchSize:    opts.BatchSize,
		LeaseSeconds: opts.LeaseSeconds,
		Concurrency:  opts.Concurrency,
		MaxPerRun:    opts.MaxPerRun,
	}, rn.process)
	stats := RunStats{
		Captured:     result.Succeeded,
		Failed:       result.Failed,
		Gone:         result.Discarded,
		DeadLettered: result.DeadLettered,
		Deferred:     result.Deferred,
	}
	if err != nil {
		return stats, fmt.Errorf("claim apply-form captures: %w", err)
	}
	return stats, nil
}

// run carries one Run's dependencies and options so process uses the receiver instead
// of threading them through every call.
type run struct {
	store     Store
	fetchers  map[string]Fetcher
	opts      RunOptions
	throttled *throttle
}

// throttle is the set of providers that answered 429 during this run. It lives for one
// run only: the next run, an hour later, asks again, which is the backoff.
type throttle struct {
	mu        sync.Mutex
	providers []string
}

// add records provider and reports whether it was new, so the run says so once.
func (t *throttle) add(provider string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if slices.Contains(t.providers, provider) {
		return false
	}
	t.providers = append(t.providers, provider)
	return true
}

func (t *throttle) has(provider string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Contains(t.providers, provider)
}

// list returns a copy, so a claim reading it never races a concurrent add.
func (t *throttle) list() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.providers)
}

// process fetches and stores one posting's form and reports what happened;
// outbox.RunPool tallies the result.
func (rn *run) process(ctx context.Context, c Claimed) outbox.Outcome {
	// Claimed in the same wave as the posting that drew the 429, before the claim could
	// know to skip it. Fetching it anyway is exactly the hammering the 429 asked us to stop.
	if rn.throttled.has(c.Provider) {
		return outbox.Deferred
	}
	err := captureOne(ctx, rn.store, rn.fetchers, rn.opts, c)
	if err == nil {
		return outbox.Succeeded
	}

	// The platform is rate-limiting us. That is about the moment, not about this posting,
	// so no attempt is charged — three throttled runs used to dead-letter thousands of
	// perfectly good captures. The lease is left in place, and the claim skips the
	// provider for the rest of the run.
	if isThrottled(err) {
		if rn.throttled.add(c.Provider) {
			log.Printf("apply-form: %s answered 429; leaving its captures for the next run", c.Provider)
		}
		return outbox.Deferred
	}

	// The platform does not have this posting and never will again. Retrying to reach
	// the same answer spends requests and ends in a dead letter that reads like a
	// fault, so the entry is retired instead.
	if errors.Is(err, ErrPostingGone) {
		if discardErr := rn.store.Discard(ctx, c.OutboxID); discardErr != nil {
			log.Printf("apply-form: retire gone capture %d: %v", c.OutboxID, discardErr)
		}
		return outbox.Discarded
	}

	dead, failErr := rn.store.Fail(ctx, c.OutboxID, err.Error(), rn.opts.MaxAttempts)
	if failErr != nil {
		log.Printf("apply-form: record failure for capture %d: %v", c.OutboxID, failErr)
	} else if dead {
		log.Printf("apply-form: capture %d (job %d) dead-lettered: %v", c.OutboxID, c.JobID, err)
	}
	if failErr == nil && dead {
		return outbox.DeadLettered
	}
	return outbox.Failed
}

// captureOne fetches and stores one posting's form.
func captureOne(ctx context.Context, s Store, fetchers map[string]Fetcher, opts RunOptions, c Claimed) error {
	// A claim for a provider with no fetcher should be impossible — the enqueue gate and
	// the fetcher registry read the same map. If one appears anyway it is failed rather
	// than skipped: skipping would leave it claimable forever, re-read by every run.
	fetcher, ok := fetchers[c.Provider]
	if !ok {
		return fmt.Errorf("no fetcher for provider %q", c.Provider)
	}
	if _, _, ok := splitBoardPosting(c.ExternalID); !ok {
		return fmt.Errorf("external id %q is not a board-namespaced posting id", c.ExternalID)
	}

	if opts.CallTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.CallTimeout)
		defer cancel()
	}

	form, err := fetcher.Fetch(ctx, c)
	if err != nil {
		return err
	}
	if err := s.Save(ctx, c.OutboxID, c.JobID, form); err != nil {
		return fmt.Errorf("store form for job %d: %w", c.JobID, err)
	}
	return nil
}
