## Why

The `/open` transparency page sources its facet distributions from the live
`/api/v1/jobs/facets` endpoint, which asks Meilisearch to count all 23 facet
attributes across the whole 3M+ catalogue on every render. That query is
~10s cold / ~1s warm, and the page's 5-minute cache guarantees a cold hit at
least that often — so `/open` regularly blocks its whole server-render on a
multi-second Meili facet count, while displaying only the top handful of values
from four facets. The figures move slowly and do not need to be live.

## What Changes

- Add a `insights_facet_stats` table holding the precomputed value→count
  distribution for the four facets `/open` displays: `countries`, `skills`,
  `seniority`, `work_mode`.
- Add a run-once-and-exit `cmd/rollup-facets` worker (mirrors `cmd/rollup-stats`)
  that recomputes the snapshot by reusing the existing `search.FacetCounts` for
  those four attributes and atomically swaps the table in one transaction. Cron
  runs it once per day.
- Add a public `GET /api/v1/stats/facets` endpoint that reads the snapshot and
  returns the same `{data: {facets: {...}}}` shape `/open` already consumes.
- Repoint `/open`'s server-side `load` from `/api/v1/jobs/facets` to
  `/api/v1/stats/facets`, removing the live Meili facet count from the page's
  render path. The live `/api/v1/jobs/facets` endpoint is unchanged — SPA filter
  facets still need it.

## Capabilities

### New Capabilities
- `facet-distribution-rollup`: a precomputed daily snapshot of selected job
  facet distributions, recomputed by a run-once worker with an atomic swap and
  exposed read-only over a public aggregate endpoint.

### Modified Capabilities
- `open-transparency-page`: the "what's inside" facet distributions are sourced
  from the precomputed `/api/v1/stats/facets` snapshot instead of the live
  `/api/v1/jobs/facets` count.

## Impact

- **New table**: `insights_facet_stats` (migration + sqlc queries).
- **New worker**: `cmd/rollup-facets` — depends on Postgres and Meilisearch
  (`MEILI_URL`/`MEILI_MASTER_KEY`); needs a daily cron entry (ops, outside this
  repo).
- **New endpoint**: `GET /api/v1/stats/facets` (public, aggregate-only).
- **Frontend**: `web/src/routes/open/+page.server.ts` swaps its facets source;
  the API client gains a `statsFacets()` method. `+page.svelte` is unchanged
  (identical data shape).
- **Unchanged**: live `/api/v1/jobs/facets` and SPA filter behaviour.
