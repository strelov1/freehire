# Similar-jobs backfill conventions

## Scope
Precomputes `GET /jobs/:slug/similar`'s data: for every job with embedding chunks
(`job_semantic_chunks`, written by `internal/embed`) but no current
`jobs.similar_job_ids`, runs the nearest-neighbour-over-chunks rollup
(`db.NearestJobsToJob` — already excludes the source job, closed jobs, and same-company
jobs) and writes the result. The full design rationale lives in the package doc comment
(`runner.go`) and `openspec/changes/drop-hybrid-search-pgvector-similar/design.md`
Decisions 3/4/5 — read those before this file, this only covers what they don't.

## Always true
- **No outbox table, unlike `internal/embed`'s `semantic_outbox`.** Finds work with a
  direct `PendingJobIDs` query (`job_semantic_chunks` exists AND
  `similar_computed_at IS NULL`) each round, not a claimed/leased queue — deliberate
  (design.md Decision 4), mirroring `../telagon`'s `cmd/similar-backfill`, this
  worker's direct ancestor. The predicate is idempotent and safe under a re-run or an
  overlapping manual invocation: a job processed twice just gets the same answer
  written twice.
- **Worker-pool concurrency (`Concurrency`/`-workers`), not `internal/outbox.RunPool`.**
  Each job's work is three independent Postgres round trips (`JobGeneration`'s read,
  the nearest-neighbour query, then a single-row conditional `UPDATE`) — no shared,
  serially-queued backend (unlike `cmd/embed`'s TEI calls) for concurrent jobs to
  contend over, so plain parallelism buys real wall-clock throughput.
- **`SetSimilarJobIDs` is a conditional write, guarded by a chunk-generation value
  (`JobGeneration`/`jobs.semantic_embedded_hash`), not a plain `UPDATE`.** Between this
  worker's read (`JobGeneration` + `NearestJobs`) and its write, `cmd/embed` can
  re-embed the same job — replacing its chunks and nulling `similar_computed_at` itself
  — in which case the computed neighbour list is already stale. The guard makes that
  write a no-op (`applied=false`, reported via `Stats.Stale`, not `Stats.Failed`) instead
  of stamping `similar_computed_at` over data the concurrent re-embed already
  invalidated; the job's real current chunks get picked up by `PendingJobIDs` on a
  later run (its `similar_computed_at` is still NULL from `cmd/embed`'s own clear).
- **In-run failure guard, not a dead-letter.** A failed job's `similar_computed_at`
  stays NULL, so it would resurface at the front of every later batch in the SAME run
  (deterministic ordering) without `Runner.Run`'s `failedThisRun` set — tracked only
  for failures, not every attempt, since a success is already excluded by the real
  predicate. Retry is time-based (the next cron firing starts with an empty set), not
  attempt-counted — there is no dead-letter state to reset or inspect.
- `cmd/embed` nulls `similar_computed_at` on every re-embed
  (`ClearSimilarComputedAtBatch`, inside `CompleteOpen`'s transaction) — that's what
  makes a plain `IS NULL` check in `PendingJobIDs` already mean "missing OR stale," no
  chunk-timestamp comparison needed.
- An empty neighbour list is a valid, intentional write (a job whose only close matches
  are same-company yields none) — it still stamps `similar_computed_at` so the job
  isn't endlessly re-picked.
- `-similar`'s default (20) matches `internal/handler`'s `maxSimilarLimit` — keep them
  in sync; a future limit increase needs this bumped (and a re-run) too, not just the
  handler.

## Limitations
None currently listed.
