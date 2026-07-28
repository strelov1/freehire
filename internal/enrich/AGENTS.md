# Enrichment conventions

## Scope
The typed enrichment contract, the LLM provider abstraction, and the queue-draining runner. The controlled vocabularies live in the neutral `internal/vocab` package (shared with the ingest and read layers).

## Always true
- The typed `Enrichment` contract in `internal/enrich` is the schema's source of truth (stored in `jobs.enrichment` JSONB; provenance in `enriched_at`/`enrichment_version`; bump `enrich.Version` to re-enrich). Enum vocabularies come from `internal/vocab` — never redefine them locally.
- `enrichment_outbox` is a reference-only queue (`job_id` + `target_version` + lease/retry bookkeeping), not a copy of the job; `jobs` stays canonical.
- Enqueue open jobs only — closed postings are skipped so a dead vacancy never burns LLM budget.
- Claims a wave of open jobs freshest-first (`ORDER BY COALESCE(posted_at, created_at) DESC, id DESC`) with `FOR UPDATE OF o SKIP LOCKED` + a `claimed_at` lease.
- **The request carries a schema derived from the `Enrichment` contract** (`schema.go`, via `internal/llmschema`), so the served enums are constrained where they are generated rather than only cleaned afterwards. Two things it deliberately does NOT do: it omits the dictionary-covered facets the prompt does not ask for (strict mode requires every property, so leaving one in would order the model to produce a value `jobview` discards), and it leaves `regions`/`countries` unconstrained — those are the dict-then-LLM discovery facets, where the prompt invites a label of the model's own and an enum would foreclose it. The schema mirrors the prompt's `askGeo` switch, so the two cannot describe different requests.
- `Enrichment.Sanitize` drops out-of-vocabulary enum values rather than dead-lettering the whole job — the invariant is "never persist an out-of-vocabulary value". **It stays on the schema path**: a gateway that stops honouring a schema answers 200 with ordinary JSON, so the schema is a first line, never a proof.
- Under a strict schema there is no absent key — an unstated field arrives as `null`, which decodes to the same zero value the omitted key produced.
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
