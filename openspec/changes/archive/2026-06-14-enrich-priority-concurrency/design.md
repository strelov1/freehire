## Context

`cmd/enrich` drains `enrichment_outbox` through a single-threaded loop
(`enrich.Runner.Run`): it claims a batch (`ClaimEnrichmentBatch`, ordered by
`outbox.id`) and processes each entry sequentially with a synchronous LLM call
(~45s each). With ~313k unenriched jobs and ingest adding more, consume rate
(~1,900/day) trails produce rate, so the backlog grows. The claim's insertion-id
order buries fresh postings behind a stale tail, and neither enqueue nor claim
excludes closed jobs, so LLM budget is spent on dead orphans.

Existing infrastructure this design leans on:

- `jobs_posted_at_id_idx (posted_at DESC NULLS LAST, id DESC)` — already serves the
  public feed's `ORDER BY`; the new claim reuses it.
- `enrichment_outbox UNIQUE (job_id, target_version)` — provides a `job_id` index for
  the claim's join probe.
- The lease mechanism (`claimed_at` + `LeaseSeconds`) doubles as the crash reaper and
  the cross-run/cross-instance dedup guard. Cron runs can overlap (no flock), so lease
  correctness is load-bearing.

## Goals / Non-Goals

**Goals:**

- Enrich fresher, open jobs first.
- Never enrich closed jobs.
- Raise throughput ~×4 via bounded concurrency, without breaking the
  one-claim-per-entry-per-run guarantee.
- No new migrations.

**Non-Goals:**

- Removing closed-job cruft already in the outbox (claim-time filter skips it; a
  cleanup sweep is a separate future change).
- Per-run caps / changing the run-till-empty drain semantics.
- Auto-tuning concurrency or adaptive rate-limiting against the LLM endpoint.
- Batching multiple jobs into one prompt.

## Decisions

### 1. Prioritize by joining `jobs` in the claim, ordering by freshness

`ClaimEnrichmentBatch` gains `JOIN jobs j ON j.id = o.job_id`,
`AND j.closed_at IS NULL`, and `ORDER BY COALESCE(j.posted_at, j.created_at) DESC, j.id DESC`.

- **Freshness = `COALESCE(posted_at, created_at)`.** `posted_at` is the user-facing
  signal (matches the feed order), but it is NULL for some sources (telegram,
  linksource, some ATS). Under a plain `posted_at DESC NULLS LAST` those undated jobs
  sort behind *every* dated job, so while ingest keeps adding dated postings during a
  multi-day backlog drain they starve indefinitely. Falling back to `created_at`
  (NOT NULL, the ingest time) ranks an undated job by recency instead — fair, and
  still "freshest first" for dated jobs.
- **`FOR UPDATE OF o SKIP LOCKED`** (not bare `FOR UPDATE`): with a join, Postgres
  row-locks every table in the `FOR UPDATE` by default. We must lock only outbox rows
  — locking `jobs` would make concurrent claim waves contend on popular job rows and
  defeats `SKIP LOCKED`'s purpose.
- *Alternative considered — `posted_at DESC NULLS LAST` (no COALESCE):* rejected for
  the starvation above. *A denormalized `posted_at` column on the outbox:* rejected;
  a column to keep in sync with no benefit over the join.

`EnqueuePendingJobs` gains `AND closed_at IS NULL` so newly closed jobs never enter
the queue. The ingest transactional enqueue (`UpsertJob`) is unchanged: a freshly
ingested or reopened job is open by construction.

### 2. Bounded-concurrency drain, claim wave sized to concurrency

`Runner.Run` keeps its outer "claim wave → drain → loop until empty" shape, but the
inner `for entry { process }` spawns one goroutine per claimed entry joined by a
`sync.WaitGroup`. Because the wave is already capped at `Concurrency` (decision below),
goroutine-per-entry *is* the bounded pool — no separate semaphore needed. `run.stats`
is guarded by a `sync.Mutex` since workers tally concurrently.

**Claim wave size = `Concurrency`** (not a separate `BatchSize`). Rationale: with a
wave of N and a pool of N, every claimed entry starts processing immediately, so an
entry's lease window ≈ one slowest LLM call (~85s) ≪ `LeaseSeconds` (300s). A larger
wave would leave tail entries leased-but-idle long enough to expire mid-drain and be
re-claimed by an overlapping cron run → double enrichment.

- *Alternative — keep `BatchSize=50`, process N-wide:* rejected; `50/4 × 45s ≈ 560s`
  drain exceeds the 300s lease, breaking the dedup guarantee under overlapping runs.
- *Alternative — continuous producer/consumer with a channel and a separate claimer
  goroutine:* rejected as over-engineered for the gain; the wave model is simpler and
  already lease-safe. (Noted as a seam if wave-boundary idle time ever matters.)

This makes `BatchSize` redundant, so `ENRICH_BATCH_SIZE` / `RunOptions.BatchSize` /
`config.Enrich.BatchSize` are removed and `Concurrency` added (default 4).

### 3. `dbStore` is already concurrency-safe

`dbStore` wraps `*pgxpool.Pool`, which is safe for concurrent use; `Job`, `Complete`,
and `Fail` each acquire their own connection. No change needed beyond the runner. The
test fake `Store`, however, must add a mutex to be race-free under the pool.

## Risks / Trade-offs

- **LLM endpoint rate-limit / 429s at concurrency 4** → default is conservative;
  `ENRICH_CONCURRENCY` is tunable per environment, and a failed call already retries
  once then counts as a failed attempt (existing behavior), so a transient 429 doesn't
  lose the entry — it's retried on a later run after the lease expires.
- **Closed-job rows accumulate as outbox cruft** → harmless (claim filter skips them);
  flagged as a known seam for a future cleanup sweep, not fixed here.
- **Concurrency hides per-entry log ordering** → each entry still logs its own
  `job=… ok/FAILED in …s` line; the per-wave heartbeat stays. Interleaving is
  acceptable for a batch worker.
- **Undated jobs (`posted_at IS NULL`)** → ranked by `created_at` via COALESCE
  (decision 1), so they are not starved.
- **Non-positive `ENRICH_CONCURRENCY`** → `LoadEnrich` floors it to 1. `LIMIT 0` would
  make the worker a silent no-op (cron looks healthy, enriches nothing) — the worst
  failure mode for an unattended worker — and a negative value would feed a bad `LIMIT`.
- **Claim no longer served by `enrichment_outbox_claimable_idx`** → the new order
  (`COALESCE(posted_at, created_at)`) can't use that partial `(id)` index, so a large
  backlog claim may sort claimable rows (or nested-loop `jobs` by `jobs_posted_at_id_idx`
  and probe the outbox `(job_id)` index). Impact is negligible because the workload is
  **LLM-latency-bound**: even a worst-case full sort per wave is <2% of the ~seconds-per-job
  LLM time it gates. No new index added (YAGNI); `EXPLAIN ANALYZE` in prod is the trigger
  to revisit if it ever becomes DB-bound.
- **Pool size vs concurrency** → `Complete` opens a short transaction per entry from
  the shared `pgxpool` (default `MaxConns = max(4, numCPU)`). At the default
  concurrency 4 this is fine; raising `ENRICH_CONCURRENCY` well above the pool size
  would serialize write-backs on `pool.Begin`. Raise the pool alongside if tuning high.

## Migration Plan

1. Edit queries → `make sqlc` → commit regenerated `internal/db`.
2. Ship code; set `ENRICH_CONCURRENCY` in the enrich worker's env (default 4 if unset).
   Remove any `ENRICH_BATCH_SIZE` from deploy config (ignored if left).
3. No DB migration. Rollback = redeploy previous binary; the outbox schema is
   unchanged, so old and new workers can even run against the same queue.

## Open Questions

None — scope settled in brainstorming.
