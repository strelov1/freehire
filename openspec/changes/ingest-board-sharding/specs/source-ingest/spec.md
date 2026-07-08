## MODIFIED Requirements

### Requirement: A standalone command runs ingest on a schedule

The system SHALL provide a standalone `cmd/ingest` binary that loads configuration, runs every configured board once with bounded concurrency, reports how many jobs were ingested and how many sources failed, and exits — suitable for scheduled execution. It SHALL accept an optional shard selector `--shard=i/n` (or the `SHARD` environment variable, both 1-based) that restricts the run to shard i of n, where distinct companies (keyed by their normalized company slug) are assigned round-robin to shards and all of a company's boards go to the same shard, so a provider whose board list is too large to finish within one timeout can be partitioned across several staggered runs that together cover the whole file. All boards of one company SHALL land in the same shard, so the per-company stale-job sweep of one shard never closes the still-live boards another shard owns. A malformed or out-of-range shard selector SHALL fail fast before any crawl, and the full board file SHALL be validated on every shard's run (not only the slice it crawls).

#### Scenario: Ingest command runs a bounded batch and exits

- **WHEN** the ingest command is run
- **THEN** it processes every configured board once and exits after reporting the ingested and failed counts

#### Scenario: A shard selector crawls only its slice

- **WHEN** the ingest command is run with `--shard=i/n` on a board file
- **THEN** it crawls only shard i's boards, and N such runs together crawl every board exactly once

#### Scenario: A company's boards are never split across shards

- **WHEN** a board file lists several boards for the same company and it is crawled with `--shard=i/n`
- **THEN** all of that company's boards fall in a single shard, so no shard's per-company stale sweep closes boards another shard owns

#### Scenario: A malformed shard selector fails fast

- **WHEN** the ingest command is run with a shard selector that is not `i/n` with `1 <= i <= n` and `n >= 1`
- **THEN** it reports the error and exits without crawling
