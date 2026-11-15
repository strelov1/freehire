// Package outbox is the shared claim-wave drain loop behind every Postgres outbox queue
// worker: claim a wave, process it, tally the outcome, repeat until a claim comes back empty.
// It owns the loop, not the queue; Runner never touches persistence itself.
package outbox

import (
	"context"
	"sync"
)

// Outcome is what a Processor did with one claimed item. The caller has already made
// whatever Store call follows from it (Complete, Fail, Discard); Runner only tallies.
type Outcome int

const (
	Succeeded Outcome = iota
	Failed
	DeadLettered
	Discarded
	// Deferred is an item put back untouched: claimed but not attempted, so no attempt is
	// counted against it (its lease expiring makes it claimable again). It still counts toward
	// MaxPerRun. Report it when the far side asked to slow down: a 429 is about the moment,
	// not the item, and spending attempts on it would dead-letter the queue.
	Deferred
)

// Stats is what one Run did.
type Stats struct {
	Succeeded    int
	Failed       int
	DeadLettered int
	Discarded    int
	Deferred     int
}

// Claimer leases up to batch live, unleased entries — the shape every package's
// existing Store.Claim method already has.
//
// Deliberately absent: Runner has no built-in context-cancellation early-exit, since callers
// differ in what a cancelled ctx should return on SIGTERM. A caller that wants early-exit
// checks ctx.Err() in its own Claim wrapper and returns (nil, nil); Runner's normal
// empty-batch stop then does the rest.
type Claimer[C any] interface {
	Claim(ctx context.Context, batch, leaseSeconds int) ([]C, error)
}

// RunOptions are the per-run knobs. MaxAttempts and per-call timeouts stay inside each
// caller's closure, not in the shared loop.
type RunOptions struct {
	BatchSize    int
	LeaseSeconds int
	// Concurrency is RunPool-only; RunBatch ignores it (its concurrency lives inside
	// whatever the caller's BatchProcessor does in one call).
	Concurrency int
	// MaxPerRun bounds how much of the backlog one Run takes, shared by RunPool and
	// RunBatch; 0 is unbounded.
	MaxPerRun int
	// OnWave, if set, is called after each wave is claimed and processed, with the cumulative
	// Stats so far (for a caller's per-wave progress log line).
	OnWave func(Stats)
}

// Processor handles one claimed item end to end, including whatever Store.Complete /
// Store.Fail / Store.Discard call follows, and reports what happened.
type Processor[C any] func(ctx context.Context, item C) Outcome

// RunPool drains via a bounded pool when opt.Concurrency > 1. At Concurrency <= 1 it runs a
// plain sequential loop with no goroutine, a real ordering guarantee for callers that need it
// (internal/application/maillink: a later message in a thread must see the wave's earlier
// link); a size-1 semaphore pool would not be FIFO.
func RunPool[C any](ctx context.Context, claimer Claimer[C], opt RunOptions, process Processor[C]) (Stats, error) {
	var stats Stats
	for {
		batchSize, budgetSpent := nextBatchSize(opt, stats)
		if budgetSpent {
			return stats, nil
		}
		batch, err := claimer.Claim(ctx, batchSize, opt.LeaseSeconds)
		if err != nil {
			return stats, err
		}
		if len(batch) == 0 {
			return stats, nil
		}
		if opt.Concurrency <= 1 {
			for _, item := range batch {
				tally(&stats, process(ctx, item))
			}
		} else {
			runPoolWave(ctx, batch, opt.Concurrency, process, &stats)
		}
		if opt.OnWave != nil {
			opt.OnWave(stats)
		}
	}
}

// BatchProcessor handles a whole wave in one call (e.g. one Meilisearch bulk push).
// A nil error means the closure fully handled every item itself, including any sub-grouping
// and per-item fallback (internal/ai/embed); the returned Stats is trusted verbatim and
// Processor is NOT also invoked. A non-nil error means the call failed outright, and RunBatch
// falls back to item-by-item Processor calls, ignoring the returned Stats.
//
// (Stats{}, nil) is deliberately meaningful: "I chose to do nothing with this wave" (e.g. the
// call timed out), leaving it claimed for a later lease-expiry retry instead of fanning out
// into many equally slow per-item calls. RunBatch tallies nothing and does not fall back.
type BatchProcessor[C any] func(ctx context.Context, items []C) (Stats, error)

// RunBatch drains via one BatchProcessor call per wave, falling back to per-item Processor
// calls only when the batch call itself fails outright, so one poison item cannot sink a
// healthy wave and a closure with its own fallback is never double-processed.
func RunBatch[C any](ctx context.Context, claimer Claimer[C], opt RunOptions, processBatch BatchProcessor[C], processOne Processor[C]) (Stats, error) {
	var stats Stats
	for {
		batchSize, budgetSpent := nextBatchSize(opt, stats)
		if budgetSpent {
			return stats, nil
		}
		batch, err := claimer.Claim(ctx, batchSize, opt.LeaseSeconds)
		if err != nil {
			return stats, err
		}
		if len(batch) == 0 {
			return stats, nil
		}
		waveStats, err := processBatch(ctx, batch)
		if err != nil {
			for _, item := range batch {
				tally(&stats, processOne(ctx, item))
			}
		} else {
			stats.Succeeded += waveStats.Succeeded
			stats.Failed += waveStats.Failed
			stats.DeadLettered += waveStats.DeadLettered
			stats.Discarded += waveStats.Discarded
			stats.Deferred += waveStats.Deferred
		}
		if opt.OnWave != nil {
			opt.OnWave(stats)
		}
	}
}

func runPoolWave[C any](ctx context.Context, batch []C, concurrency int, process Processor[C], stats *Stats) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, concurrency)

	for _, item := range batch {
		wg.Add(1)
		go func(it C) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			outcome := process(ctx, it)
			mu.Lock()
			tally(stats, outcome)
			mu.Unlock()
		}(item)
	}
	wg.Wait()
}

// nextBatchSize applies opt.MaxPerRun to opt.BatchSize, reporting whether the budget is
// already spent, in which case the caller must stop without claiming again.
func nextBatchSize(opt RunOptions, stats Stats) (size int, budgetSpent bool) {
	if opt.MaxPerRun <= 0 {
		return opt.BatchSize, false
	}
	processed := stats.Succeeded + stats.Failed + stats.DeadLettered + stats.Discarded + stats.Deferred
	remaining := opt.MaxPerRun - processed
	if remaining <= 0 {
		return 0, true
	}
	return min(opt.BatchSize, remaining), false
}

func tally(stats *Stats, o Outcome) {
	switch o {
	case Succeeded:
		stats.Succeeded++
	case Failed:
		stats.Failed++
	case DeadLettered:
		stats.DeadLettered++
	case Discarded:
		stats.Discarded++
	case Deferred:
		stats.Deferred++
	}
}
