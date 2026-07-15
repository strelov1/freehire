# Enrichment conventions

## Scope
The typed enrichment contract, controlled vocabularies, the LLM provider abstraction, and the queue-draining runner.

## Always true
- The typed `Enrichment` contract + controlled vocabularies in `internal/enrich` are the schema's source of truth (stored in `jobs.enrichment` JSONB; provenance in `enriched_at`/`enrichment_version`; bump `enrich.Version` to re-enrich).
- `enrichment_outbox` is a reference-only queue (`job_id` + `target_version` + lease/retry bookkeeping), not a copy of the job; `jobs` stays canonical.
- Enqueue open jobs only — closed postings are skipped so a dead vacancy never burns LLM budget.
- Claims a wave of open jobs freshest-first (`ORDER BY COALESCE(posted_at, created_at) DESC, id DESC`) with `FOR UPDATE OF o SKIP LOCKED` + a `claimed_at` lease.
- `Enrichment.Sanitize` drops out-of-vocabulary enum values rather than dead-lettering the whole job — the invariant is "never persist an out-of-vocabulary value".
- `Validate` as a guard: an LLM/parse error retries once, then dead-letters.
- On success, writes via `SetJobEnrichment` + deletes the outbox row in one transaction.
- `SetJobEnrichment` is deliberately separate from `UpsertJob` so ingest and enrichment stay decoupled.
- Never hard-code a vendor or model — the LLM is configured by `LLM_BASE_URL`/`LLM_API_KEY`/`LLM_MODEL` (any OpenAI-compatible endpoint).
- The lease expiry is the built-in reaper — no separate process.
- Overlapping cron runs can't double-enrich: the wave is sized to the concurrency so an entry's lease window stays ≈ one LLM call.

## How it works
`cmd/enrich` enqueues pending rows (open jobs only), then repeatedly claims a wave of open jobs freshest-first with `FOR UPDATE OF o SKIP LOCKED` + a `claimed_at` lease, and drains each wave concurrently across `ENRICH_CONCURRENCY` workers (default 4). Undated jobs fall back to ingest time so they don't starve. It enriches via the `Provider` (LLM behind an interface; swap the impl, don't couple callers) under a per-call timeout so a stalled gateway can't hang the worker. `Enrichment.Sanitize` cleans out-of-vocabulary enum values (drops the stray field rather than dead-lettering the whole job), then `Validate`s as a guard. On success it writes via `SetJobEnrichment` + deletes the outbox row in one transaction.

## Limitations
None currently listed.
