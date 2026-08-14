// Package similarjobs computes each job's precomputed "similar jobs" list — the
// nearest-neighbour-over-chunks rollup (internal/db's already-built NearestJobsToJob)
// written once per job into jobs.similar_job_ids/similar_computed_at, so GET
// /jobs/:slug/similar becomes a plain indexed Postgres lookup instead of a live vector
// query (openspec/changes/drop-hybrid-search-pgvector-similar/design.md Decisions
// 3/4/5). cmd/similar-backfill wires the concrete adapter.
//
// Unlike internal/embed's semantic_outbox, this worker finds outstanding work with a
// direct "what's missing" query rather than a claimed/leased queue (design.md Decision
// 4 — telagon's cmd/similar-backfill, the original inspiration for this whole
// worker, has no outbox table either: it just loops "find jobs missing a result,
// process them, repeat"). The predicate (a job has embedding chunks but no current
// similar_job_ids) is idempotent and re-orderable, so a batched full-table scan needs
// no lease/dead-letter machinery to stay safe under a re-run or an overlapping manual
// invocation — a job processed twice just gets the same answer written twice.
package similarjobs

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Store is the persistence the runner needs. The real implementation wraps the
// generated queries + pool; tests use a fake.
type Store interface {
	// PendingJobIDs returns up to limit job ids that have at least one
	// job_semantic_chunks row but no precomputed similar-jobs list yet
	// (similar_computed_at IS NULL), ordered freshest-job-first (mirrors
	// cmd/embed's claim ordering — newly-posted jobs get a similar-jobs list
	// before older backlog on a still-draining catalogue). cmd/embed nulls
	// similar_computed_at on every re-embed (see its ClearSimilarComputedAtBatch
	// call inside CompleteOpen), so a plain IS NULL check already captures
	// "missing OR stale" — no chunk-timestamp comparison needed. Returns an empty
	// slice, not an error, once nothing is outstanding.
	PendingJobIDs(ctx context.Context, limit int) ([]int64, error)
	// JobGeneration returns jobID's current chunk-generation marker (the source
	// job's semantic_embedded_hash) — read alongside NearestJobs and passed back to
	// SetSimilarJobIDs as a conditional-write guard against a concurrent cmd/embed
	// re-embed racing between the two (see SetSimilarJobIDs).
	JobGeneration(ctx context.Context, jobID int64) (string, error)
	// NearestJobs returns up to limit candidate job ids nearest to jobID, nearest
	// first — wraps db.NearestJobsToJob, which already excludes the source job,
	// closed jobs, and jobs from the source job's own company. An empty result is
	// valid: a job whose only close matches are same-company yields none, not an
	// error.
	NearestJobs(ctx context.Context, jobID int64, limit int) ([]int64, error)
	// SetSimilarJobIDs writes one job's computed neighbour list and stamps
	// similar_computed_at = now() together, so a job is never marked computed
	// without its list landing. A nil/empty similarJobIDs is a valid, intentional
	// write (see NearestJobs) — it still stamps the job so it is not endlessly
	// re-picked by PendingJobIDs.
	//
	// generation must be the value JobGeneration returned for this job before
	// NearestJobs ran. The write is conditional on jobID's CURRENT
	// semantic_embedded_hash still matching it: if cmd/embed replaced this job's
	// chunks (and its own ClearSimilarComputedAtBatch call already nulled
	// similar_computed_at) between that read and this write, the computed
	// neighbour list reflects chunks that no longer exist, and applied comes back
	// false instead of stamping over data the concurrent re-embed already
	// invalidated — the job is left for a later run to redo against its new chunks.
	SetSimilarJobIDs(ctx context.Context, jobID int64, similarJobIDs []int64, generation string) (applied bool, err error)
}

// RunOptions are the per-run knobs.
type RunOptions struct {
	// BatchSize is PendingJobIDs' page size — how many candidate jobs the runner
	// looks at per round trip before checking in on progress.
	BatchSize int
	// NeighbourCount is how many neighbours to request (and store) per job. Set to
	// at least the API's maxSimilarLimit (internal/handler, 20 as of this
	// writing) so a future increase to that cap doesn't require a re-backfill of
	// already-computed jobs.
	NeighbourCount int
	// Concurrency is how many jobs are processed in parallel within a batch. Each
	// job's work is three independent Postgres round trips (JobGeneration's
	// single-row read, NearestJobsToJob's chunk-join query, then a single-row
	// conditional UPDATE) — not a call to an external index or embedder the way
	// cmd/embed's IndexOpen is, so there is no shared,
	// serially-queued backend for concurrent jobs to contend over. Parallelizing
	// across jobs (mirroring telagon's own cmd/similar-backfill
	// worker-pool-over-a-channel shape, the direct ancestor of this worker) buys
	// real wall-clock throughput on what is otherwise Postgres/CPU-bound,
	// per-job-independent work. Values < 1 are treated as 1 (no parallelism).
	Concurrency int
	// MaxJobs bounds how many jobs a single run attempts; 0 means unlimited
	// (drain until PendingJobIDs returns nothing new).
	MaxJobs int
}

// Stats reports what a run did.
type Stats struct {
	Processed int // jobs whose similar_job_ids was successfully written (possibly an empty list)
	Failed    int // jobs that errored and were left for a later run to retry
	Stale     int // jobs whose write lost the SetSimilarJobIDs generation race (see Store) — not
	// an error: cmd/embed's own concurrent re-embed already left the job's
	// similar_computed_at NULL, so PendingJobIDs naturally re-selects it (this run or
	// the next) against its now-current chunks.
}

// Runner drives the process: repeatedly fetch a batch of jobs needing work, compute
// and write each one's neighbour list, until nothing new remains or MaxJobs is hit.
type Runner struct {
	Store Store
}

// Run drains outstanding work. A failure on one job is logged and counted, never
// aborts the run — see the failedThisRun comment below for why a failure doesn't
// resurface within the SAME run. It does resurface on the next scheduled run: this
// worker keeps no dead-letter state, retry is time-based (the next cron firing), not
// attempt-counted (design.md Decision 4 — no outbox machinery).
func (r Runner) Run(ctx context.Context, opt RunOptions) (Stats, error) {
	concurrency := opt.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	// failedThisRun guards against an in-run infinite loop: a job that failed (or
	// lost the SetSimilarJobIDs generation race — see Stats.Stale) still has
	// similar_computed_at NULL, so PendingJobIDs' predicate does not change for it
	// — without this guard it would resurface at the front of every subsequent
	// batch (the query's ordering is deterministic) and the run would never
	// terminate. Only these are tracked, not every attempted id: a
	// successfully-processed job is already excluded by the real predicate (its
	// similar_computed_at is no longer NULL), so re-adding it here would just
	// grow this set toward the full catalogue size (~1.6-2M jobs, per design.md)
	// for zero benefit on the common all-succeeds path. A fresh process (the next
	// cron firing) retries any failure/stale race with a fresh, empty set.
	failedThisRun := map[int64]bool{}
	var stats Stats

	for opt.MaxJobs <= 0 || stats.Processed+stats.Failed+stats.Stale < opt.MaxJobs {
		ids, err := r.Store.PendingJobIDs(ctx, opt.BatchSize)
		if err != nil {
			return stats, fmt.Errorf("pending job ids: %w", err)
		}
		if len(ids) == 0 {
			break
		}

		// Filter out ids that already failed this run — a safe in-place filter
		// since fresh's write index never runs ahead of ids' read index.
		fresh := ids[:0]
		for _, id := range ids {
			if !failedThisRun[id] {
				fresh = append(fresh, id)
			}
		}
		if len(fresh) == 0 {
			// Every candidate PendingJobIDs returned this round already failed
			// earlier in this same run — nothing left this run can make
			// progress on.
			break
		}
		if opt.MaxJobs > 0 {
			if remaining := opt.MaxJobs - (stats.Processed + stats.Failed + stats.Stale); remaining < len(fresh) {
				fresh = fresh[:remaining]
			}
		}

		processed, staleIDs, failedIDs := r.processBatch(ctx, fresh, opt.NeighbourCount, concurrency)
		for _, id := range staleIDs {
			failedThisRun[id] = true
		}
		for _, id := range failedIDs {
			failedThisRun[id] = true
		}
		stats.Processed += processed
		stats.Stale += len(staleIDs)
		stats.Failed += len(failedIDs)
		log.Printf("similarjobs: progress processed=%d stale=%d failed=%d", stats.Processed, stats.Stale, stats.Failed)
	}
	return stats, nil
}

// processBatch computes+writes similar_job_ids for a batch of jobs, up to
// `concurrency` in flight at once — a worker pool over a channel (mirroring
// telagon's cmd/similar-backfill shape), not internal/outbox's RunPool, which
// claims/leases/dead-letters entries: this worker finds work by direct query, not an
// outbox (design.md Decision 4). Returns the ids that lost the generation race
// (Store.SetSimilarJobIDs) and the ids that errored, so the caller can add both to its
// in-run failure guard.
func (r Runner) processBatch(ctx context.Context, ids []int64, neighbourCount, concurrency int) (processed int, staleIDs, failedIDs []int64) {
	if concurrency > len(ids) {
		concurrency = len(ids)
	}
	work := make(chan int64, len(ids))
	for _, id := range ids {
		work <- id
	}
	close(work)

	var mu sync.Mutex
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range work {
				applied, err := r.processOne(ctx, id, neighbourCount)
				mu.Lock()
				switch {
				case err != nil:
					log.Printf("similarjobs: job %d: %v", id, err)
					failedIDs = append(failedIDs, id)
				case !applied:
					staleIDs = append(staleIDs, id)
				default:
					processed++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return processed, staleIDs, failedIDs
}

// processOne computes and writes one job's similar-jobs list. applied is false, not an
// error, when Store.SetSimilarJobIDs lost the generation race — see its doc comment.
func (r Runner) processOne(ctx context.Context, jobID int64, neighbourCount int) (applied bool, err error) {
	generation, err := r.Store.JobGeneration(ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("job generation: %w", err)
	}
	neighbours, err := r.Store.NearestJobs(ctx, jobID, neighbourCount)
	if err != nil {
		return false, fmt.Errorf("nearest jobs: %w", err)
	}
	applied, err = r.Store.SetSimilarJobIDs(ctx, jobID, neighbours, generation)
	if err != nil {
		return false, fmt.Errorf("set similar job ids: %w", err)
	}
	return applied, nil
}
