package adzunadesc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
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

// RunOptions are the per-run knobs, mirroring internal/applyform's shape.
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

// RunStats is what the run did.
type RunStats struct {
	Captured int
	Failed   int
	// Skipped counts claims discarded for errURLDrifted — not a failure of the fetch, so
	// not retried and not dead-lettered.
	Skipped      int
	DeadLettered int
}

// Degraded reports whether a run deserves an alert: work abandoned (a dead letter) or a run
// that failed everything it touched. Mirrors applyform.RunStats.Degraded — a handful of
// transient failures per run is this worker's normal, healthy shape too.
func (s RunStats) Degraded() bool {
	return s.DeadLettered > 0 || (s.Captured == 0 && s.Failed > 0)
}

// Run drains the capture queue and returns. It claims a wave, fetches each posting's
// description, stores it, and keeps going until a wave comes back empty.
//
// It returns an error only when the QUEUE itself is unusable — there is no per-item
// recovery from being unable to claim. Every other failure belongs to one capture and is
// recorded against it.
func Run(ctx context.Context, s Store, fetch FetchFunc, opts RunOptions) (RunStats, error) {
	var stats RunStats

	for {
		batch := opts.BatchSize
		if opts.MaxPerRun > 0 {
			remaining := opts.MaxPerRun - (stats.Captured + stats.Failed + stats.Skipped)
			if remaining <= 0 {
				return stats, nil
			}
			batch = min(batch, remaining)
		}

		claims, err := s.Claim(ctx, batch, opts.LeaseSeconds)
		if err != nil {
			return stats, fmt.Errorf("claim adzuna description captures: %w", err)
		}
		if len(claims) == 0 {
			return stats, nil
		}

		wave := runWave(ctx, s, fetch, opts, claims)
		stats.Captured += wave.Captured
		stats.Failed += wave.Failed
		stats.Skipped += wave.Skipped
		stats.DeadLettered += wave.DeadLettered

		if ctx.Err() != nil {
			return stats, nil
		}
	}
}

// runWave processes one claim wave through a bounded pool.
func runWave(ctx context.Context, s Store, fetch FetchFunc, opts RunOptions, claims []Claimed) RunStats {
	concurrency := max(opts.Concurrency, 1)

	var (
		mu    sync.Mutex
		stats RunStats
		wg    sync.WaitGroup
	)
	slots := make(chan struct{}, concurrency)

	for _, c := range claims {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			err := captureOne(ctx, s, fetch, opts, c)
			if err == nil {
				mu.Lock()
				stats.Captured++
				mu.Unlock()
				return
			}

			if errors.Is(err, errURLDrifted) {
				if discardErr := s.Discard(ctx, c.OutboxID); discardErr != nil {
					log.Printf("adzunadesc: discard drifted capture %d: %v", c.OutboxID, discardErr)
				}
				mu.Lock()
				stats.Skipped++
				mu.Unlock()
				return
			}

			// A terminal error (see fetch.go) will answer the same way on a retry, so it
			// dead-letters on this first attempt regardless of opts.MaxAttempts — no point
			// spending two more requests to learn the same thing twice.
			maxAttempts := opts.MaxAttempts
			if terminal(err) {
				maxAttempts = 1
			}
			dead, failErr := s.Fail(ctx, c.OutboxID, err.Error(), maxAttempts)
			if failErr != nil {
				log.Printf("adzunadesc: record failure for capture %d: %v", c.OutboxID, failErr)
			} else if dead {
				log.Printf("adzunadesc: capture %d (job %d) dead-lettered: %v", c.OutboxID, c.JobID, err)
			}

			mu.Lock()
			defer mu.Unlock()
			stats.Failed++
			if failErr == nil && dead {
				stats.DeadLettered++
			}
		}()
	}
	wg.Wait()

	return stats
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
