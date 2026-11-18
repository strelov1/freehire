package adzunadesc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/outbox"
)

// errURLDrifted marks a claim whose job's stored URL has moved to the ad-network tracking
// redirect since it was enqueued — not a fetch failure, so it is never retried.
var errURLDrifted = errors.New("adzunadesc: job's url drifted to the ad-network redirect since enqueue")

// Claimed is one capture leased to this run.
type Claimed struct {
	OutboxID int64
	JobID    int64
	// URL is the posting's own hosted page — the same one Eligible already accepted
	// before this row ever reached the queue.
	URL string
}

// Store is the persistence the runner needs. The real implementation wraps the generated
// queries and a pool; tests use a fake.
type Store interface {
	// Claim leases up to batch live, unleased captures.
	Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error)
	// Save stores the fetched description and retires the queue entry, atomically — so a
	// capture cannot be both stored and left queued, nor retired without being stored.
	Save(ctx context.Context, outboxID, jobID int64, description string) error
	// Fail records a failed attempt for one entry; it reports whether it dead-lettered.
	Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (deadLettered bool, err error)
	// Discard retires an entry without storing anything and without counting an attempt —
	// for a claim errURLDrifted rejects, which will never succeed by retrying because the
	// URL itself is now wrong, not the fetch. Deleting rather than dead-lettering leaves the
	// job free to be re-enqueued the next time a crawl finds it eligible again.
	Discard(ctx context.Context, outboxID int64) error
}

// FetchFunc reads one posting's page and returns its full, sanitized description.
type FetchFunc func(ctx context.Context, url string) (string, error)

// RunOptions are the per-run knobs, mirroring internal/ingest/applyform's shape.
type RunOptions struct {
	BatchSize    int
	LeaseSeconds int
	MaxAttempts  int
	Concurrency  int
	// MaxPerRun bounds how much of the backlog ONE run takes; 0 is unbounded. See
	// applyform.RunOptions.MaxPerRun for why this, not just Concurrency, is what keeps a
	// systemd Type=oneshot worker from silently swallowing scheduled firings behind a
	// long-running catch-up.
	MaxPerRun   int
	CallTimeout time.Duration
}

// RunStats is what the run did. Failed and DeadLettered are mutually exclusive — a
// dead-lettered capture counts only in DeadLettered, matching internal/platform/outbox.Outcome's
// one-outcome-per-item shape (and applyform.RunStats, migrated onto the same shared
// runner just before this package: see its doc comment for the pre-migration
// double-count this convention corrects).
type RunStats struct {
	Captured int
	Failed   int
	// Skipped counts claims discarded for errURLDrifted — not a failure of the fetch, so
	// not retried and not dead-lettered.
	Skipped      int
	DeadLettered int
}

// Degraded reports whether a run deserves an alert: one that captured nothing while it
// failed or abandoned what it touched. A dead letter beside captures is this queue's normal
// shape, not a fault: Adzuna drops the JobPosting block from a posting it has taken down, so
// 3-30% of every wave dead-letters on errNoJobPosting (measured 2026-11-05), and counting
// that turned the unit red on every run, which is the same as never alerting.
func (s RunStats) Degraded() bool {
	return s.Captured == 0 && (s.Failed > 0 || s.DeadLettered > 0)
}

// Run drains the capture queue and returns. It claims a wave, fetches each posting's
// description, stores it, and keeps going until a wave comes back empty.
//
// It returns an error only when the QUEUE itself is unusable — there is no per-item
// recovery from being unable to claim. Every other failure belongs to one capture and is
// recorded against it.
func Run(ctx context.Context, s Store, fetch FetchFunc, opts RunOptions) (RunStats, error) {
	rn := &run{store: s, fetch: fetch, opts: opts}
	result, err := outbox.RunPool(ctx, &cancelAwareClaimer{store: s}, outbox.RunOptions{
		BatchSize:    opts.BatchSize,
		LeaseSeconds: opts.LeaseSeconds,
		Concurrency:  opts.Concurrency,
		MaxPerRun:    opts.MaxPerRun,
	}, rn.process)
	stats := RunStats{
		Captured:     result.Succeeded,
		Failed:       result.Failed,
		Skipped:      result.Discarded,
		DeadLettered: result.DeadLettered,
	}
	if err != nil {
		return stats, fmt.Errorf("claim adzuna description captures: %w", err)
	}
	return stats, nil
}

// cancelAwareClaimer adapts Store to outbox.Claimer, checking ctx.Err() before every
// claim AFTER the first — see internal/ingest/applyform's identical adapter (this package
// mirrors it) for the full rationale, including why the first call is deliberately
// unguarded: the original loop never checked ctx before its first claim, only between
// waves, so an already-cancelled ctx at Run()'s very start still made one real claim
// attempt (a visible error) rather than a silent zero-stats no-op.
type cancelAwareClaimer struct {
	store Store
	calls int
}

func (c *cancelAwareClaimer) Claim(ctx context.Context, batch, leaseSeconds int) ([]Claimed, error) {
	if c.calls > 0 && ctx.Err() != nil {
		return nil, nil
	}
	c.calls++
	return c.store.Claim(ctx, batch, leaseSeconds)
}

// run carries one Run's dependencies and options so process uses the receiver instead
// of threading them through every call.
type run struct {
	store Store
	fetch FetchFunc
	opts  RunOptions
}

// process fetches and stores one posting's description and reports what happened;
// outbox.RunPool tallies the result.
func (rn *run) process(ctx context.Context, c Claimed) outbox.Outcome {
	err := captureOne(ctx, rn.store, rn.fetch, rn.opts, c)
	if err == nil {
		return outbox.Succeeded
	}

	if errors.Is(err, errURLDrifted) {
		if discardErr := rn.store.Discard(ctx, c.OutboxID); discardErr != nil {
			log.Printf("adzunadesc: discard drifted capture %d: %v", c.OutboxID, discardErr)
		}
		return outbox.Discarded
	}

	// A terminal error (see fetch.go) will answer the same way on a retry, so it
	// dead-letters on this first attempt regardless of opts.MaxAttempts — no point
	// spending two more requests to learn the same thing twice.
	maxAttempts := rn.opts.MaxAttempts
	if terminal(err) {
		maxAttempts = 1
	}
	dead, failErr := rn.store.Fail(ctx, c.OutboxID, err.Error(), maxAttempts)
	if failErr != nil {
		log.Printf("adzunadesc: record failure for capture %d: %v", c.OutboxID, failErr)
	} else if dead {
		log.Printf("adzunadesc: capture %d (job %d) dead-lettered: %v", c.OutboxID, c.JobID, err)
	}
	if failErr == nil && dead {
		return outbox.DeadLettered
	}
	return outbox.Failed
}

// captureOne fetches and stores one posting's description.
func captureOne(ctx context.Context, s Store, fetch FetchFunc, opts RunOptions, c Claimed) error {
	// Re-check the URL shape against what the claim carries NOW, not what it was at
	// enqueue time — see errURLDrifted.
	if !detailsPageURL(c.URL) {
		return errURLDrifted
	}
	if opts.CallTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.CallTimeout)
		defer cancel()
	}

	description, err := fetch(ctx, c.URL)
	if err != nil {
		return err
	}
	if err := s.Save(ctx, c.OutboxID, c.JobID, description); err != nil {
		return fmt.Errorf("store description for job %d: %w", c.JobID, err)
	}
	return nil
}
