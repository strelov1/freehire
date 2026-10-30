# source-quality-metrics Specification

## Purpose
TBD - created by archiving change add-source-quality-metrics. Update Purpose after archive.
## Requirements
### Requirement: Per-source quality counts

The per-source snapshot SHALL record, for each source, how many of its open, non-private
postings have no work mode, no country and no region, no company, have gone unseen for
more than 7 days, and were posted more than 60 days ago.

#### Scenario: Counted in the existing scan

- **WHEN** `cmd/rollup-stats` rebuilds `source_stats`
- **THEN** the five counts come from the same grouped scan that measures `open_jobs`
- **AND** a source with no open posting records zero for each

### Requirement: Published as gauges

`cmd/queue-metrics` SHALL publish each count as `freehire_source_quality_gap{source,check}`,
each source's `open_jobs` as `freehire_source_open_jobs{source}`, and the snapshot's
measurement time as `freehire_source_stats_measured_timestamp_seconds`.

#### Scenario: Empty snapshot

- **WHEN** `source_stats` holds no row
- **THEN** none of the three families is published

