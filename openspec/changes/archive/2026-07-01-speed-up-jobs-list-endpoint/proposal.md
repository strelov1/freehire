## Why

`GET /api/v1/jobs` (the DB-backed public jobs list) takes 17–30s. Measured
against prod and isolated from network latency (its TTFB dwarfs the `/health`
network floor, unlike every other endpoint), the cause is two Postgres queries
over the ~2.5M open-job set on every call:

- `ListJobs`: `... WHERE closed_at IS NULL ORDER BY created_at DESC, id DESC
  LIMIT/OFFSET` has **no supporting index** (only `jobs_posted_at_id_idx` and a
  `company_slug` index exist), so Postgres scans and sorts the entire open set.
- `CountJobs`: `count(*) WHERE closed_at IS NULL` is a full count of ~2.5M rows.

The endpoint is **not used by the web UI** (the browse and company pages use
`/jobs/search` via Meilisearch; `listJobs()` is unused and no `/api/v1/jobs?`
consumer exists in the repo). It is a documented public REST endpoint, so this is
a latency landmine for external API consumers, not a user-facing app path.

## What Changes

- Add a partial index matching the list order so `ListJobs` becomes an index scan
  with no sort: `CREATE INDEX CONCURRENTLY jobs_open_created_idx ON jobs
  (created_at DESC, id DESC) WHERE closed_at IS NULL`.
- Replace the exact `CountJobs` full count with a **fast planner-based estimate**
  of the open-job total, so the endpoint's `meta.total` no longer blocks on
  counting millions of rows. **BREAKING (minor, this endpoint only)**: `/api/v1/jobs`
  `meta.total` becomes approximate. `/jobs/search` (which already reports an
  *estimated* total from Meili) and all UI surfaces are unchanged.
- No change to `/jobs/search`, the browse/company pages, response envelope, job
  wire shape, or the closed-job filtering.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `job-search`: the DB-backed `GET /api/v1/jobs` list is served via a partial
  index (no full-table sort) and its `meta.total` is an approximate estimate
  rather than an exact count — replacing the prior "the existing `GET /api/v1/jobs`
  list endpoint SHALL be unchanged" clause.

## Impact

- **Schema**: new migration adding `jobs_open_created_idx` (partial index).
  `CREATE INDEX CONCURRENTLY` cannot run inside a transaction, and initdb only
  runs on first volume init, so this is applied **manually on prod** (documented
  in tasks); the migration file remains the source of truth for a fresh DB.
- **Backend**: `internal/db/queries/jobs.sql` (`CountJobs` → an estimate query),
  regenerated `internal/db`; `internal/handler/jobs.go` (use the estimate).
- **Tests**: sqlc regeneration; an integration test (testcontainers,
  `-tags=integration`) covering the list ordering and that a total is returned.
- **No reindex, no Meili change, no API envelope change.**
