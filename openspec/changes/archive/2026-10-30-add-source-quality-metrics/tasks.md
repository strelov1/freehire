## 1. Measure

- [x] 1.1 Migration `0213_source_stats_quality.sql`: five nullable `bigint` (NULL = not yet measured)
  columns on `source_stats`. `pnpm check:sql`.
- [x] 1.2 Extend `AggregateOpenJobsBySource` with the five `FILTER` counts and
  `InsertSourceStat` with the columns; `make sqlc`.
- [x] 1.3 `sourcestats.Rows` carries the counts through (zero for a source absent from the
  scan); unit test.

## 2. Publish

- [x] 2.1 `SourceQualityMetrics` query in `metrics.sql`; `cmd/queue-metrics` collects it.
- [x] 2.2 Render `freehire_source_open_jobs`, `freehire_source_quality_gap{source,check}`,
  `freehire_source_stats_measured_timestamp_seconds`; pin the text in `render_test.go`.
- [x] 2.3 Integration test: the aggregate counts each check against seeded rows.

## 3. Alert (freehire-ops)

- [x] 3.1 Rule: open_jobs >= 500 and a check's share rose by 20+ points over 7 days.
- [x] 3.2 Rule: the snapshot is older than 12 hours.
