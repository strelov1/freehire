## Why

Nothing measures whether what a source delivers is any GOOD. `board_health` and the
provider families in `cmd/queue-metrics` answer "is the crawl failing"; the ingest-volume
pair answers "is it writing copies". A spike on 2026-10-29 read the open catalogue per
source and found, unalerted:

- `successfactors`: ~98% of open postings carry an empty location — no geography at all.
- `hrmdirect`: every crawl succeeds, nothing has been listed since 2026-10-21, and 100% of
  its open postings have gone unseen for 7+ days while still being shown.
- `hrmos`: Japanese locations the location dictionary does not parse.
- `eures`: a quarter of its open postings carry no company.

What matters to a visitor is that the remote/on-site signal is there and that what we
show is still open. Both are measurable per source from columns we already store.

## What Changes

- The per-source scan `cmd/rollup-stats` already runs every 3 hours
  (`AggregateOpenJobsBySource`) also counts, per source, the open postings that:
  have no `work_mode`, have no country and no region, have no company, have gone unseen
  by their crawl for 7+ days, and were posted more than 60 days ago.
- `source_stats` stores those five counts beside `open_jobs`.
- `cmd/queue-metrics` publishes them as `freehire_source_quality_gap{source,check}`, with
  `freehire_source_open_jobs{source}` as the denominator and the snapshot's measurement
  time, so a dead rollup is visible.
- Alert rules (in `freehire-ops`) fire on a source's share JUMPING, not on its level.

## Impact

- Migration 0213 (five nullable `bigint` columns, NULL = not yet measured on a ~400-row table).
- `internal/platform/db/queries/source_stats.sql`, `metrics.sql`; `internal/ingest/sourcestats`;
  `cmd/queue-metrics`.
- No new scan of `jobs`, no new worker, no public API change.
