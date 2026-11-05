package searchdrain

import (
	"context"
	"fmt"
	"log"
)

// Drain alternates deletion and indexing waves with independent per-queue budgets.
// A short claim ends that queue's pass, allowing incoming trickles to accumulate
// until the next scheduled run instead of submitting many tiny index mutations.
func Drain(ctx context.Context, indexing Runner, removals DeletionRunner, opt RunOptions) (Stats, DeletionStats, error) {
	var indexed Stats
	if opt.BatchSize < 1 {
		return indexed, DeletionStats{}, fmt.Errorf("search-drain: batch size must be positive")
	}
	indexed.Reaped = reap(ctx, indexing.Store)
	var deleted DeletionStats
	indexDone, deleteDone := false, false
	for !indexDone || !deleteDone {
		if err := ctx.Err(); err != nil {
			return indexed, deleted, err
		}
		// Between waves, because a backlog makes one run last hours: a rebuild reaching its
		// bulk load has to stop this run, not just the next one. Whatever is queued stays
		// queued, and the rebuild's swap reflects it.
		if opt.Stop != nil && opt.Stop() {
			log.Printf("search-drain: paused, stopping after indexed=%d deleted=%d", indexed.Indexed, deleted.Deleted)
			return indexed, deleted, nil
		}
		if !deleteDone {
			waveOpt := waveOptions(opt, deleted.Claimed)
			stats, err := removals.run(ctx, waveOpt, true)
			deleted.Claimed += stats.Claimed
			deleted.Deleted += stats.Deleted
			deleted.Failed += stats.Failed
			deleted.DeadLettered += stats.DeadLettered
			if err != nil {
				return indexed, deleted, err
			}
			deleteDone = stats.Claimed < waveOpt.BatchSize || (opt.MaxPerRun > 0 && deleted.Claimed >= opt.MaxPerRun)
		}
		if !indexDone {
			waveOpt := waveOptions(opt, indexed.Claimed)
			stats, err := indexing.run(ctx, waveOpt, true)
			indexed.Claimed += stats.Claimed
			indexed.Indexed += stats.Indexed
			indexed.Failed += stats.Failed
			indexed.DeadLettered += stats.DeadLettered
			indexed.Reaped += stats.Reaped
			if err != nil {
				return indexed, deleted, err
			}
			indexDone = stats.Claimed < waveOpt.BatchSize || (opt.MaxPerRun > 0 && indexed.Claimed >= opt.MaxPerRun)
		}
		log.Printf("search-drain: progress indexed=%d deleted=%d failed=%d dead=%d",
			indexed.Indexed, deleted.Deleted, indexed.Failed+deleted.Failed, indexed.DeadLettered+deleted.DeadLettered)
	}
	return indexed, deleted, nil
}

func waveOptions(opt RunOptions, claimed int) RunOptions {
	if opt.MaxPerRun > 0 {
		opt.BatchSize = min(opt.BatchSize, opt.MaxPerRun-claimed)
	}
	opt.MaxPerRun = opt.BatchSize
	return opt
}
