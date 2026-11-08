## Why

The aggregator has an enrichment worker and a job read API but **no way to get
jobs in** — there are no source parsers and no ingest pipeline, so the catalogue
only holds hand-seeded rows. We need a modular ingest path where adding a parser
for a new source is cheap (one file), so the catalogue can fill from real job
boards.

## What Changes

- Add `internal/sources/`: a small parser framework. A `Source` adapter implements
  one ATS platform (Greenhouse, Lever, Ashby); each is constructed once and looked
  up by provider key. Adding a platform is a new file plus one line in the explicit
  `All()` constructor. Adding a company is a config edit, no code.
- Add a `sources.yml` config listing the boards to crawl (`company`, `provider`,
  `board`), loaded at `cmd/ingest` startup. An unknown provider fails fast.
- Add `internal/pipeline/`: the write path. It dispatches each configured board to
  its adapter, normalizes the result, and persists it. `source` is the platform;
  `external_id` is namespaced `"<board>:<ats-id>"` so the dedup key
  `UNIQUE (source, external_id)` can't collide across companies on one platform.
- Add `cmd/ingest`: a thin standalone binary (mirrors `cmd/enrich`) that loads the
  config, runs every enabled board with bounded concurrency, isolates per-source
  failures, and exits. Run on a schedule.
- **BREAKING (internal write path)** `UpsertJob` stops overwriting the enrichment
  columns on conflict, so a re-ingest **preserves** existing enrichment instead of
  wiping it. Ingest carries no enrichment payload; the enrichment worker remains the
  only writer of those columns.
- Ingest enqueues each upserted job into `enrichment_outbox` in the **same
  transaction** as the upsert (transactional outbox), but only when the job is not
  yet enriched to the current `enrich.Version` — so new jobs get queued atomically
  without re-enriching already-done ones.

## Capabilities

### New Capabilities

- `source-ingest`: how jobs enter the catalogue — the modular source-adapter
  contract, config-driven board selection, the normalize/dedup write path, the
  namespaced dedup key, per-source failure isolation, and the standalone ingest
  command.

### Modified Capabilities

- `ai-enrichment`: the durable outbox is now also fed transactionally by the ingest
  write path (previously: backfill-only from provenance), gated on the job not
  already being enriched to the target version.

## Impact

- **New code**: `internal/sources/` (interface, registry, HTTP client, three
  adapters, config loader), `internal/pipeline/` (runner + Store interface),
  `cmd/ingest/` (binary + DB-backed Store), `sources.yml`.
- **DB access**: `queries/jobs.sql` — `UpsertJob` no longer touches enrichment
  columns on conflict; new enqueue query inserting into `enrichment_outbox` gated on
  enrichment provenance. Regenerate `internal/db` via `make sqlc`.
- **No schema change**: reuses existing `jobs` and `enrichment_outbox` tables; no
  new migration.
- **Out of scope**: incremental/delta fetch, per-source rate limiting, headless
  browsers, per-job description detail-fetch (these three platforms carry the
  description in their list endpoints), DB-backed source config, platform auth, and
  re-enrichment on content change (bump `enrich.Version` to force re-enrich).
