## Context

`/open` renders server-side and, in its `load`, fans out to several public
endpoints via `Promise.allSettled`; the whole render blocks on the slowest leg.
That leg is `/api/v1/jobs/facets`, which makes Meilisearch count all 23 facet
attributes over the 3M+ catalogue (~10s cold / ~1s warm). The page then displays
only the top values of four facets. `/open` already sets `max-age=300`, so a cold
Meili facet count recurs at least every five minutes.

The codebase already has the pattern this change needs: `cmd/rollup-stats`
recomputes `job_daily_stats` and the `insights_*` tables as run-once workers,
each an atomic delete-and-reinsert inside one transaction, cron-scheduled. The
one difference here is the data *source*: those rollups are SQL aggregates over
`jobs`; this snapshot's authoritative source is Meilisearch's facet count, which
is exactly what the live filters and the current `/open` consume — so reusing
`search.FacetCounts` guarantees the snapshot's numbers match the live catalogue
and the SPA filter facets.

## Goals / Non-Goals

**Goals:**
- Remove the live Meili facet count from `/open`'s render path.
- Precompute the four facets `/open` shows (`countries`, `skills`, `seniority`,
  `work_mode`) once per day, served from Postgres over a public endpoint.
- Guarantee snapshot numbers equal the live facet counts (same vocabulary).
- Keep the live `/api/v1/jobs/facets` and SPA filtering unchanged.

**Non-Goals:**
- No change to how SPA filter facets (disjunctive, arbitrary filters) are counted.
- No caching/HTTP/2/compression work on `/open` (tracked separately).
- No snapshot of the other 19 facets — only the four `/open` renders.
- No per-filter or time-scoped facet snapshots; the snapshot is the unfiltered
  whole-catalogue distribution.

## Decisions

**1. Reuse `search.FacetCounts`, compute before the transaction.**
The worker builds a Meili client (`search.NewClient(cfg.MeiliURL, cfg.MeiliKey)`,
as `cmd/reindex` does), calls `FacetCounts` for the four attributes, and only
*then* opens a short DB transaction to swap the table. Computing the source
before `BEGIN` means a slow/failing Meili never holds a transaction open, and a
Meili error aborts the run before it touches the table.
_Alternative — SQL `GROUP BY` over `jobs` (like the insights rollups):_ avoids
the Meili dependency, but would re-implement the skills/countries normalization
that lives in the search index and risk the snapshot diverging from the live
filter facets. Rejected for the parity risk.

**2. Store the full distribution, slice top-N on read.**
`insights_facet_stats(facet text, value text, count bigint, PRIMARY KEY(facet,
value))`. `/open` shows top-8/8/6/3, but storing the full distribution keeps the
table a plain projection of the facet count, lets the endpoint (or future
callers) choose N, and costs little. Mirrors the insights tables' plain shape.

**3. Atomic delete-and-reinsert in one transaction.**
Same discipline as `cmd/rollup-stats`: `DeleteAllFacetStats` + batched
`InsertFacetStat` inside one `tx`, commit at the end. Readers keep the prior
snapshot until commit; reruns are idempotent; a failed rebuild rolls back and the
worker exits non-zero for cron to alert. No `SET LOCAL work_mem` needed — the
insert set is tiny (a few hundred rows), not a full-catalogue aggregate.

**4. Public `GET /api/v1/stats/facets`, same shape as today.**
A new handler reads `ListFacetStats`, groups rows into
`{data:{facets:{<facet>:{<value>:<count>}}}}` — the exact shape `/open`'s
`+page.svelte` already consumes from `api.facetCounts`. Registered next to the
other public `/stats/*` reads. The frontend gains `api.statsFacets()` and
`+page.server.ts` swaps its facets leg; `+page.svelte` is untouched.

**5. Separate daily cron.**
Run `cmd/rollup-facets` on its own once-a-day schedule rather than folding it
into `rollup-stats`. The two have different data sources (Meili vs Postgres) and
different natural cadences (`rollup-stats` runs intra-day for same-day activity
freshness; facet distributions move slowly enough for daily). Cron wiring lives
in ops, outside this repo — flagged in tasks.

## Risks / Trade-offs

- **Snapshot staleness (up to 24h).** → Acceptable: the `/open` figures are
  explicitly "numbers that move slowly"; a day-old distribution is fine, and the
  headline scale/activity/growth stats stay live from their own endpoints.
- **Meili unavailable during the daily run** → the run aborts non-zero without
  touching the table (compute-before-transaction), so the prior snapshot serves
  on; cron alerts on the non-zero exit.
- **Empty snapshot before first run (fresh deploy / fresh volume)** → the
  endpoint returns 200 with empty facet maps, and `/open`'s existing per-section
  degradation shows the "distribution unavailable" fallback until the first run.
- **New migration on existing prod volume** → per the repo's initdb gotcha, the
  `CREATE TABLE` must be run manually before deploying code that reads it;
  captured in the migration plan.

## Migration Plan

1. Add migration `00NN_insights_facet_stats.sql` (initdb applies it on a fresh
   volume; on prod, run the `CREATE TABLE` manually before the code deploy).
2. Deploy the worker + endpoint + frontend change together (endpoint tolerates an
   empty table, so ordering with the first worker run is not load-bearing).
3. Add the daily cron entry for `cmd/rollup-facets` (ops).
4. Run the worker once manually to populate the snapshot, then verify `/open`
   sources `/api/v1/stats/facets`.

**Rollback:** revert `+page.server.ts` to `api.facetCounts` (live `/jobs/facets`
is untouched); the new table/worker/endpoint can stay dormant.

## Open Questions

None — data source, storage, read path, and cadence are settled.
