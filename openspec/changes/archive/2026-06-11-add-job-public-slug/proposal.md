## Why

Public job endpoints expose the internal sequential `BIGINT` `id`
(`/api/v1/jobs/123`), which lets anyone enumerate the whole catalogue by walking
`1..N` and infer inventory size and fill rate from the id growth. A public
aggregator should expose a non-enumerable, human-readable identifier instead,
without sacrificing the narrow internal integer key used for joins.

## What Changes

- Add a `public_slug` column to `jobs` (`TEXT NOT NULL UNIQUE`), generated
  deterministically from `title`, `company`, and a short code derived from the
  dedup key `(source, external_id)`. Determinism keeps the slug stable across
  re-ingests so external links/SEO/bookmarks don't break.
- `jobs.id` stays internal: it remains the `BIGINT GENERATED ALWAYS AS IDENTITY`
  primary key and the foreign key target for `user_jobs.job_id`. Not exposed.
- **BREAKING** Public job routes switch their path parameter from the numeric
  `:id` to `:slug`: `GET /api/v1/jobs/:slug`, `POST /api/v1/jobs/:slug/view`,
  `POST /api/v1/jobs/:slug/apply`. The interaction handlers resolve the slug to
  the internal `id` before writing `user_jobs`.
- `UpsertJob` computes and writes the slug as part of the existing single write;
  slug generation reuses `internal/normalize` (already used for `company_slug`).

## Capabilities

### New Capabilities

- `job-public-identity`: how a job is identified to the outside world — the
  public slug format, its stability/uniqueness guarantees, and slug-based public
  routing for read and per-user interaction endpoints.

### Modified Capabilities
<!-- No existing capability owns the public job read/interaction endpoints yet
     (no jobs spec under openspec/specs/), so this is purely additive. -->

## Impact

- **Schema**: new `migrations/` file adding `jobs.public_slug` (column + UNIQUE).
  No versioned migration runner yet — applies on first initdb; existing volumes
  need recreation (`docker compose down -v && make up`).
- **DB access**: `queries/jobs.sql` — `UpsertJob` writes `public_slug`; new
  `GetJobBySlug`. Regenerate `internal/db` via `make sqlc`.
- **Handlers**: `handler` job read + `user_jobs` view/apply switch to `:slug`
  and resolve to the internal id.
- **New code**: deterministic slug builder (in `internal/normalize` or alongside
  the upsert write path).
- **Out of scope**: `external_id` from a source's native id (no parsers/ingest
  yet); the short code is computed from whatever `(source, external_id)` the
  current backfill rows already carry.
