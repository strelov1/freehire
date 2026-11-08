## Context

Phase 1 (`add-job-enrichment-schema`, archived) shipped the target schema: a typed
`Enrichment` struct + controlled vocabularies in `internal/enrich`, a
`jobs.enrichment JSONB` column, and provenance columns (`enriched_at`,
`enrichment_version`). The package deliberately contains *no* AI calls — only the
contract. This change is phase 2: the layer that calls an LLM to fill that contract
from `jobs.description` and writes it back, organized around a durable enrichment
queue.

Constraints carried from the repo: Go + Fiber + sqlc (no ORM); migrations apply via
Postgres initdb only (no versioned runner yet); `internal/db/*.go` is generated;
`UpsertJob` is the pipeline's existing write path. The pipeline and source parsers do
not exist yet, so enrichment must be independently runnable against rows already in
`jobs`.

Decisions below were settled with the user: standalone command trigger; a durable
outbox queue (with an eye to offloading enrich execution to an external queue later);
LangChainGo over an OpenAI-compatible endpoint; provider-agnostic config; delete the
queue entry on success; lease-based claim with dead-lettering.

## Goals / Non-Goals

**Goals:**

- Populate `jobs.enrichment` for queued jobs from their description, constrained to the
  phase-1 vocabularies, with a row-level provenance stamp.
- Make the work a durable queue (`enrichment_outbox`) so it survives restarts, supports
  safe concurrency, and can later be drained by an external queue without reshaping the
  data model.
- Never persist an out-of-vocabulary payload; bound wasted spend on hopeless rows.
- Keep the LLM behind a small interface so the call site is testable and the provider
  is swappable without touching the command or DB layer.

**Non-Goals:**

- The ingest pipeline / source parsers (separate future change). This change enqueues
  existing rows via a backfill; ingest will become the in-transaction producer later.
- An external message broker (SQS/Rabbit) or worker fleet — the outbox is the seam for
  that; we do not build it now.
- A long-running daemon/scheduler — `cmd/enrich` is one-shot, scheduled externally.
- Per-field confidence or source attribution beyond the row-level stamp (phase-1
  non-goal, still out).
- A versioned migration runner — one additive migration ships here under the existing
  initdb constraint.

## Decisions

### D1. A durable `enrichment_outbox` queue, jobs stays canonical

Work is tracked in a new `enrichment_outbox` table: one row per
`(job_id, target_version)` needing enrichment, referencing the job by id — **not** a
copy of the job. `jobs` remains the single source of truth; the outbox is only a work
queue + bookkeeping (`attempts`, `claimed_at`, `failed_at`, `last_error`,
`created_at`), with `UNIQUE (job_id, target_version)`.

- **Why a reference queue, not a staging copy:** a copy of the job would duplicate the
  source of truth and force sync between two tables. A reference (id + version) does
  not — it is purely a queue. This sidesteps the duplication objection while giving a
  durable, inspectable handoff point.
- **Why over provenance-only selection on `jobs`:** provenance columns can express
  "pending" (`enriched_at IS NULL`), but a dedicated queue gives per-item attempts,
  leasing, and dead-lettering, and — the deciding factor — is the clean seam for later
  offloading enrich execution to an external queue. The producer just inserts a row;
  the consumer can be this command today or a broker-driven worker tomorrow.
- **Alternative (full transactional outbox to an external bus):** deferred — there is
  no external bus yet. The table is the durable substrate that makes adding one later
  non-breaking.

### D2. Producer: backfill now, in-transaction ingest later

Two producer paths:

- **Now:** `EnqueuePendingJobs(target_version)` — `INSERT ... SELECT` from `jobs` where
  `enriched_at IS NULL OR enrichment_version < target_version`, `ON CONFLICT (job_id,
  target_version) DO NOTHING`. Idempotent; safe to run every command invocation.
- **Later (seam):** the ingest path inserts into `enrichment_outbox` in the **same
  transaction** as the job upsert — the transactional-outbox guarantee that a newly
  ingested job is never lost from enrichment.
- **Honest caveat:** the transactional-outbox payoff (atomic ingest+enqueue) only
  materializes when ingest exists. Until then the backfill bridges existing rows. We
  accept building the queue ahead of its in-transaction producer because it is the
  data-model seam the user wants for the future queue, and the backfill keeps it useful
  today.

### D3. Lease-based claim with `SKIP LOCKED`; built-in reaper

The consumer claims a bounded batch in a short transaction:

```sql
WITH claimed AS (
  SELECT id FROM enrichment_outbox
  WHERE failed_at IS NULL
    AND (claimed_at IS NULL OR claimed_at < now() - @lease_seconds * interval '1 second')
  ORDER BY id FOR UPDATE SKIP LOCKED LIMIT @batch
)
UPDATE enrichment_outbox o SET claimed_at = now()
FROM claimed WHERE o.id = claimed.id
RETURNING o.id, o.job_id, o.target_version, o.attempts;
```

The LLM call happens **outside** any transaction — the claim only stamps `claimed_at`,
so no row lock or DB connection is held across the slow call.

- **Why claimed_at + lease over lock-through-the-call:** holding `FOR UPDATE` across a
  multi-second LLM call would pin a connection and lock per in-flight job — an
  anti-pattern at any real throughput. Leasing releases the lock immediately.
- **Why no separate reaper process:** the lease predicate in the claim query *is* the
  reaper — a crashed worker's entries (stale `claimed_at`) become claimable again after
  `lease_seconds`. One fewer moving part.
- **`SKIP LOCKED`** makes N concurrent workers (or overlapping cron runs) pick disjoint
  entries, so "don't process the same row twice" holds without external locking.

### D4. Delete on success; retry-once then dead-letter on failure

- **Success** (payload passes `Enrichment.Validate`): one transaction writes
  `jobs.enrichment` + `enriched_at = now()` + `enrichment_version = target_version`
  (via `SetJobEnrichment`) **and** deletes the outbox entry. The table then holds only
  pending + dead-lettered rows.
- **Failure** (validation fails twice in a row, or the LLM errors): increment
  `attempts`, record `last_error`, and leave `claimed_at` in place; when `attempts`
  reaches the configured max, set `failed_at` (dead-letter) so it is never claimed
  again. An invalid payload is **never** written to `jobs`. The lease is *not* cleared
  on failure — its expiry gates the retry to a later run and doubles as the crash
  reaper, so a failed entry is never reprocessed within the same run (no hot loop).
- **Why delete on success:** the user chose a lean table; pending + dead-letter only.
  Provenance on `jobs` already records that the job was enriched and at what version, so
  the outbox needs no "done" history.
- **Why dead-letter over infinite retry:** a permanently-unparseable description would
  otherwise burn tokens every run. `failed_at` rows are surfaceable for inspection.

### D5. LangChainGo provider over an OpenAI-compatible endpoint, behind `enrich.Provider`

Extraction goes through `github.com/tmc/langchaingo` using its OpenAI-compatible client
pointed at a configurable base URL (`openai.WithBaseURL(LLM_BASE_URL)`), behind:

```go
type Provider interface {
    Enrich(ctx context.Context, job JobInput) (Enrichment, error)
}
```

`JobInput` carries the raw source fields the LLM reads. The command depends on the
interface, not on LangChainGo — a fake provider drives unit tests, and a different
backend can replace the implementation without touching `cmd/enrich` or the DB layer.

- **Why LangChainGo (user choice):** provider portability and a familiar API; the
  OpenAI-compatible client + base URL is what makes "any provider behind a gateway"
  work without provider-specific code. Trade-off accepted (see Risks).

### D6. Provider-agnostic config

`LLM_BASE_URL` (OpenAI-compatible endpoint — LiteLLM gateway, a Chinese model provider,
etc.), `LLM_API_KEY`, `LLM_MODEL`. No vendor name or default model is hard-coded; the
command fails fast if any is unset.

- **Why no vendor-specific keys or default:** the user routes through a gateway and
  Chinese models. A vendor-named key or default model would leak a vendor assumption
  into code; routing/model choice is an ops concern the gateway + these three env vars
  cover. There is no provider-neutral "sensible default" model, so requiring the three
  surfaces misconfiguration immediately.

### D7. `enrich.Version`, two new query files, one migration

- `const Version = 1` in `internal/enrich` — stamped on write, used as `target_version`
  for enqueue/selection. Bumping it enqueues re-enrichment.
- `migrations/0004_enrichment_outbox.sql` creates the table + a partial claim index
  (`WHERE failed_at IS NULL`).
- `internal/db/queries/enrichment.sql`: `EnqueuePendingJobs`, `ClaimEnrichmentBatch`,
  `RecordEnrichmentFailure`, `DeleteEnrichmentEntry`; plus `SetJobEnrichment` in
  `jobs.sql`. `UpsertJob` is left alone — ingest's full-upsert path stays decoupled
  from enrichment's targeted update.

## Risks / Trade-offs

- **Outbox payoff is partly deferred (no in-transaction producer yet)** → until ingest
  exists, atomic ingest+enqueue is not exercised. Mitigation: the idempotent backfill
  keeps the queue correct and useful today; the table/queries are the exact substrate
  ingest will reuse.
- **Building queue infra ahead of strict need** → more code (migration, queries,
  claim/lease) than provenance-only selection. Mitigation: accepted per the user's
  explicit goal of a future external queue; the cost is bounded and the seam is real.
- **Structured output over OpenAI-compatible backends relies on function-calling / JSON
  mode, reliability varies by model** → no token-level grammar guarantee. Mitigation:
  D4's validate + retry-once + dead-letter guarantees no invalid value is persisted; the
  `Provider` seam (D5) lets us swap the implementation or pin a stricter `LLM_MODEL`.
- **Heavy transitive dependency tree (LangChainGo ~170+ deps)** → larger build /
  supply-chain surface for a service holding an LLM key. Mitigation: accepted per user
  decision; the `Provider` seam keeps the blast radius to one package.
- **Lease tuning** → too short re-dispatches still-running jobs (double spend); too long
  slows crash recovery. Mitigation: make `lease_seconds` (and batch size, max attempts)
  configurable; start conservative.
- **Migration applies via initdb only** → existing volumes won't get
  `enrichment_outbox` without `down -v && make up`; production needs the migration-runner
  seam before persistent rollout (unchanged, reaffirmed).
- **LLM cost scales with queue depth** → mitigated by the cheap configurable model,
  once-per-version enqueue, dead-lettering, and the batch limit.

## Migration Plan

1. Add `migrations/0004_enrichment_outbox.sql` (table + partial claim index).
2. Add `internal/db/queries/enrichment.sql` queries and `SetJobEnrichment` in
   `jobs.sql`; run `make sqlc`; commit generated code.
3. Add `enrich.Version`, `Provider` interface, `JobInput`, and the LangChainGo
   implementation in `internal/enrich`.
4. Extend `internal/config` with `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`.
5. Add `cmd/enrich`: config → pool → provider → enqueue → claim/enrich/write loop.
6. Dev: `docker compose down -v && make up` to re-init the volume with the new table.
7. Rollback: drop the migration, the two query files' additions, the provider, and
   `cmd/enrich`. The change is additive (new table + three enrichment columns already
   present); no data migration needed — enriched rows simply stop being updated.

## Open Questions

- None blocking. Two deferred levers: parallelism (run multiple `cmd/enrich`
  concurrently — already safe via `SKIP LOCKED`, just unscheduled) and offloading the
  consumer to an external queue (the outbox is the seam; not built now). Lease duration,
  batch size, and max attempts ship as configurable values with conservative defaults.
