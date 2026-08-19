## Why

Three dedup passes write one column, `jobs.duplicate_of`, and the first of them recomputes
it from scratch. `RecomputeRoleDuplicatesForCompanies` derives the marker purely from
`role_fingerprint` clusters and writes NULL to every row that is a canon or a singleton in
its own cluster — including rows the aggregator-suppression and fuzzy passes had marked for
entirely different reasons. Those two passes then re-apply their markers later in the same
run. The end state is correct; the cost is that every run rewrites hundreds of thousands of
rows that were already correct before it started.

Measured on prod over 2026-08-16..18, ten consecutive runs, six per day:

| Pass | Rows re-marked per run |
|---|---|
| role duplicates | 293k–495k (typically ~460k) |
| aggregator suppression | 120k–128k |
| fuzzy collapse | 305k–348k |

The variance is a few percent across three days. A genuinely changing catalogue would track
ingest volume and the marked population would accumulate; a flat number equal to the
population is the signature of a full wipe followed by a full re-apply. Each pass carries an
`IS DISTINCT FROM` guard, so these counts are rows whose stored value actually changed.

The current specs already encode the contradiction. `fuzzy-description-role-dedup` requires
the pass to be "idempotent and reversible by the standard recompute" and separately requires
that "the second run changes no `duplicate_of` markers". Both hold in isolation. Neither
survives a scheduler that runs the recompute immediately before the fuzzy pass on every
cycle, which is what `cmd/reindex`'s `refreshDuplicateMarkers` does.

Three consequences make this worth fixing now rather than noting:

1. **Write churn.** ~950k row updates per cycle x 6 cycles = ~5.7M writes a day that change
   nothing, on a 7.4M-row table. Dead tuples, bloat, and autovacuum load — the same
   mechanism that previously made the companies count drift.
2. **`updated_at` noise.** Every churned row gets `updated_at = now()`, so `reindex --since`
   and every other consumer of "changed since" sees a third of the catalogue as changed.
3. **A window where duplicates are live.** Between the recompute and the passes that repair
   it, the database states that hundreds of thousands of duplicates are canonical. On
   2026-08-18 the role pass finished at 20:21, the fuzzy pass restored 346,483 markers at
   21:45, and the scheduled facet rebuild started at 21:16 — inside the window. Whatever it
   scanned in those 29 minutes reached the live index unmarked.

The third consequence also blocks planned work: moving duplicate markers onto the existing
`search_outbox` / `search_delete_outbox` queues, so the facet index no longer needs a
scheduled full rebuild, is not viable while a cycle produces ~950k marker changes.

## What Changes

- Each dedup pass gets its own marker column and writes only that column:
  `duplicate_of_role`, `duplicate_of_aggregator`, `duplicate_of_fuzzy`. A pass can no longer
  clear a marker it did not set.
- `jobs.duplicate_of` becomes derived from the three, resolved in a fixed precedence, so
  every existing reader — job search, the facet index, embeddings, enrichment, cluster
  copies — keeps reading the column it reads today with unchanged semantics.
- `refreshDuplicateMarkers` stops depending on pass order for correctness. Ordering may stay
  as a cost optimization (the fuzzy pass still only considers rows the earlier passes left
  canonical), but a run that fails or is interrupted between passes no longer leaves
  duplicates unmarked.
- Re-running the marker refresh with no catalogue change writes zero rows, which is what the
  existing `IS DISTINCT FROM` guards were meant to deliver and what the fuzzy spec already
  requires.
- **BREAKING** for direct writers of `jobs.duplicate_of`: the column stops being directly
  writable. `MarkJobDuplicateOf` and the ingest-time clustering write that sets the marker on
  insert must be routed to an owning column.

## Capabilities

### New Capabilities

- `duplicate-marker-ownership`: which pass owns which marker, how the three resolve into the
  single `duplicate_of` value readers consume, and the guarantee that a marker refresh is
  idempotent and order-independent.

### Modified Capabilities

- `fuzzy-description-role-dedup`: the requirement that the pass be "reversible by the
  standard recompute" is replaced by ownership — the recompute no longer reaches the fuzzy
  marker at all. The existing "re-running is stable" scenario is strengthened to hold across
  a full refresh cycle, not only across two consecutive fuzzy passes.
- `aggregator-ats-dedup`: suppression and release become the aggregator pass's exclusive
  authority over its own marker; the role recompute clearing a suppression is specified as
  incorrect rather than expected.

## Impact

- **Schema:** a migration adding three columns to `jobs` and turning `duplicate_of` into a
  derived value, plus a backfill that seeds the owning columns from current markers. `jobs`
  is 7.4M rows on prod, so the migration must be written for that scale and must not rewrite
  the table under a lock — see `design.md`.
- **Queries:** `internal/db/queries/jobs.sql` — `RecomputeRoleDuplicatesForCompanies`,
  `SuppressAggregatorDuplicatesForCompanies`, `MarkFuzzyDuplicatesForCompany`,
  `MarkJobDuplicateOf`, and every read predicate on `duplicate_of` (search-outbox claim,
  semantic outbox, enrichment, pruning, cluster copies).
- **Workers:** `cmd/reindex` (`refreshDuplicateMarkers` and both dedup entry points),
  `cmd/search-drain` claim eligibility.
- **Not affected:** the public job wire shape, the facet document, and every HTTP handler —
  they read a job's duplicate status, not the column layout.
- **Unblocks:** enqueuing marker changes onto `search_outbox` / `search_delete_outbox`, which
  is what would let the scheduled full facet rebuild become an on-demand operation.
