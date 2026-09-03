## MODIFIED Requirements

### Requirement: Boards to crawl are configured in a database table

The system SHALL read the set of boards to crawl from the `boards` table, scoped to the
single provider named by the `cmd/ingest` invocation. Each row carries `company`,
`provider`, and — for a provider that has a board/tenant concept — `board`, already
validated at write time (see `board-catalog`), so `cmd/ingest` need not re-validate
provider existence or board shape before crawling. Only a row whose `status` is `pending`
or `active` SHALL be crawled; a `rejected` or `retired` row SHALL be excluded from every
run.

#### Scenario: Boards are loaded for the invoked provider

- **WHEN** `cmd/ingest` is invoked naming provider `greenhouse`
- **THEN** it crawls exactly the `boards` rows with `provider = 'greenhouse'` and
  `status` in (`pending`, `active`)

#### Scenario: A retired board is not crawled

- **WHEN** a board's `status` is `retired`
- **THEN** the run excludes it without treating its absence as an error

#### Scenario: A rejected board is not crawled

- **WHEN** a board's `status` is `rejected`
- **THEN** the run excludes it from every provider run

### Requirement: A standalone command runs ingest on a schedule

The system SHALL provide a standalone `cmd/ingest` binary that takes a provider name,
loads that provider's `pending`/`active` rows from the `boards` table, runs every one once
with bounded concurrency, reports how many jobs were ingested and how many boards failed,
and exits — suitable for scheduled execution. It SHALL accept an optional shard selector
`--shard=i/n` (or the `SHARD` environment variable, both 1-based) that restricts the run
to shard i of n, where distinct companies (keyed by their normalized company slug) are
assigned round-robin to shards and all of a company's boards go to the same shard, so a
provider whose board list is too large to finish within one timeout can be partitioned
across several staggered runs that together cover every one of that provider's boards.
All boards of one company SHALL land in the same shard, so the per-company stale-job
sweep of one shard never closes the still-live boards another shard owns. A malformed or
out-of-range shard selector SHALL fail fast before any crawl.

#### Scenario: Ingest command runs a bounded batch and exits

- **WHEN** the ingest command is run for a provider
- **THEN** it processes every one of that provider's eligible boards once and exits after
  reporting the ingested and failed counts

#### Scenario: A shard selector crawls only its slice

- **WHEN** the ingest command is run with `--shard=i/n` for a provider
- **THEN** it crawls only shard i's boards, and N such runs together crawl every board of
  that provider exactly once

#### Scenario: A company's boards are never split across shards

- **WHEN** a provider has several boards for the same company and it is crawled with
  `--shard=i/n`
- **THEN** all of that company's boards fall in a single shard, so no shard's per-company
  stale sweep closes boards another shard owns

#### Scenario: A malformed shard selector fails fast

- **WHEN** the ingest command is run with a shard selector that is not `i/n` with
  `1 <= i <= n` and `n >= 1`
- **THEN** it reports the error and exits without crawling
