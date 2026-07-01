## Context

`GET /api/v1/jobs` (handler `ListJobs`, `internal/handler/jobs.go`) runs two
Postgres queries per request over the ~2.5M open-job set:

- `ListJobs`: `SELECT * FROM jobs WHERE closed_at IS NULL ORDER BY created_at
  DESC, id DESC LIMIT $1 OFFSET $2`. The only jobs indexes are
  `jobs_posted_at_id_idx (posted_at DESC NULLS LAST, id DESC)` and
  `(company_slug, posted_at …)` — neither matches a `created_at` order under the
  `closed_at IS NULL` filter, so Postgres scans and sorts the whole open set.
- `CountJobs`: `SELECT count(*) FROM jobs WHERE closed_at IS NULL` — a full count
  of ~2.5M rows.

Measured against prod, the endpoint's TTFB is 17–30s while every other endpoint
sits at the network floor (~0.77s from a laptop). The endpoint is unused by the
web UI (browse/company pages use `/jobs/search` via Meilisearch; `listJobs()` is
unused and no `/api/v1/jobs?` consumer exists in the repo) — it is a public REST
list endpoint, so this is a latency landmine for API consumers.

## Goals / Non-Goals

**Goals:**
- `GET /api/v1/jobs` responds fast (sub-100ms server-side) at catalogue scale.
- The list page is served through an index (no per-request full sort).
- `meta.total` is produced in O(1), not by counting millions of rows.

**Non-Goals:**
- Touching `/jobs/search`, the browse/company pages, or the response envelope.
- A versioned migration runner (still absent — see the manual-apply note).
- Exact `meta.total` for this endpoint (it becomes an approximate estimate).

## Decisions

### Partial index matching the list order

Add `CREATE INDEX jobs_open_created_idx ON jobs (created_at DESC, id DESC) WHERE
closed_at IS NULL`. It exactly matches `ListJobs`' filter + order, so the query
becomes an index scan that reads only `LIMIT+OFFSET` entries — no sort.

- **Fresh DB:** the migration file uses a plain `CREATE INDEX IF NOT EXISTS`
  (instant on the empty initdb table; it is also the sqlc schema source, matching
  the existing `jobs_posted_at_id_idx` style in `0001_init.sql`).
- **Prod:** applied **manually** as `CREATE INDEX CONCURRENTLY` to avoid an
  exclusive lock on the live 2.5M-row table. `CONCURRENTLY` cannot run inside a
  transaction, so it is not put in the migration file (which initdb/psql may wrap)
  — the file's plain form is for fresh volumes, the concurrent form is the
  documented prod step. Alternative rejected: putting `CONCURRENTLY` in the file
  risks breaking initdb's transaction handling.

### Approximate `meta.total` via the Postgres planner estimate

Replace the exact `count(*)` with the planner's estimated row count for `WHERE
closed_at IS NULL`, wrapped in a narrow SQL function:

```sql
CREATE FUNCTION estimate_open_jobs() RETURNS bigint AS $$
DECLARE plan json;
BEGIN
  EXECUTE 'EXPLAIN (FORMAT json) SELECT 1 FROM jobs WHERE closed_at IS NULL'
    INTO plan;
  RETURN (plan->0->'Plan'->>'Plan Rows')::bigint;
END; $$ LANGUAGE plpgsql;
```

`CountJobs` is renamed to `EstimateOpenJobs` (`SELECT estimate_open_jobs()::bigint`)
so the name reflects that the value is now an estimate; the handler switches to it.

- **Why the planner estimate over alternatives:**
  - `count(*)` even with the partial index is an index-only scan whose cost is
    linear in the open-set size (~2.5M entries) — still hundreds of ms and still
    O(catalogue). Rejected.
  - `pg_class.reltuples` is O(1) but counts the **whole** table (open + closed),
    so it over-reports when closed jobs are non-trivial. Rejected for accuracy.
  - The planner estimate is O(1) (planning only, no execution) **and** filtered by
    `closed_at IS NULL` using `pg_statistic`, so it tracks the open-set size.
- **Narrow, not generic:** the function EXECUTEs a **constant** query, so there is
  no arbitrary-SQL/injection surface (rejected a generic `count_estimate(text)`).
- **Accuracy:** the estimate is as good as autovacuum/ANALYZE stats — sufficient
  for an approximate list total. The spec now calls this total approximate.

## Risks / Trade-offs

- **Estimate drift after bulk ingests** → autovacuum refreshes stats; a stale
  estimate is acceptable for an approximate total and self-heals. Mitigation: none
  needed; document that it is approximate.
- **Prod index build** → `CREATE INDEX CONCURRENTLY` is slower but non-locking;
  run it off-peak. If it fails midway it leaves an INVALID index to drop and
  retry (standard `CONCURRENTLY` caveat) — documented in tasks.
- **Test determinism** → on a tiny test table the planner estimate need not equal
  the real count, so the integration test asserts the estimate query runs and
  returns a non-negative bigint, and asserts the real behavior (open-only,
  `created_at`/`id` ordering) on `ListJobs`, rather than an exact estimate value.
- **sqlc + plpgsql function** → sqlc parses the migration for schema; the function
  is DDL it ignores, and the `::bigint` cast in the query fixes the return type.
- **Deep-offset pagination stays O(offset)** → the index removes the *sort*, not
  the OFFSET walk: an index scan still steps over `OFFSET` entries before
  returning `LIMIT`, so a very deep offset still reads many index tuples. The
  "sub-100ms" goal is therefore scoped to typical offsets (the common page-0..N
  access pattern); deep-offset paging over millions is inherent to offset
  pagination and out of scope (keyset paging exists separately for the sitemap).
