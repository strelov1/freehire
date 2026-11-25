## ADDED Requirements

### Requirement: Queue depth is published per outbox

The pipeline SHALL publish, for each of `search_outbox`, `enrichment_outbox`, and
`semantic_outbox`, the number of live entries, the number of dead-lettered entries, and
the age of the oldest live entry, as Prometheus gauges labelled by queue name.

An entry is live when `failed_at IS NULL` and dead-lettered when `failed_at IS NOT NULL`.
Age is measured from the entry's `created_at`.

#### Scenario: A queue with live and dead-lettered entries

- **WHEN** an outbox holds live entries and dead-lettered entries
- **THEN** `freehire_queue_depth` for that queue MUST count only the live entries
- **AND** `freehire_queue_dead_letters` for that queue MUST count only the dead-lettered entries
- **AND** `freehire_queue_oldest_age_seconds` for that queue MUST report the age of the oldest live entry

#### Scenario: An empty queue

- **WHEN** an outbox holds no entries at all
- **THEN** `freehire_queue_depth` and `freehire_queue_dead_letters` for that queue MUST both be published as `0`
- **AND** `freehire_queue_oldest_age_seconds` for that queue MUST be published as `0`

A queue that has drained must publish an explicit zero rather than omitting the series:
an absent series is indistinguishable from a dead exporter, and alert rules treat the
two differently.

### Requirement: Board fleet health is published by state

The pipeline SHALL publish the size of the ingest board fleet broken down by health
state, as a Prometheus gauge labelled `state` with the values `healthy`, `failing`, and
`cooled`.

A board is `cooled` when its `cooldown_until` is in the future, `failing` when it is not
cooled and its `consecutive_failures` is above zero, and `healthy` otherwise. The three
states are mutually exclusive and MUST sum to the total number of boards.

#### Scenario: A fleet spanning all three states

- **WHEN** the board fleet contains boards in cooldown, boards with failures but no active cooldown, and boards with neither
- **THEN** each board MUST be counted in exactly one state
- **AND** the three published values MUST sum to the total number of rows in the board fleet

#### Scenario: A board both failing and cooled

- **WHEN** a board has a non-zero `consecutive_failures` and a `cooldown_until` in the future
- **THEN** it MUST be counted as `cooled` only, and MUST NOT also be counted as `failing`

### Requirement: Catalogue freshness is published

The pipeline SHALL publish the creation time of the most recently created job as a Unix
timestamp gauge, so a consumer can derive how long the catalogue has gone without new
postings.

#### Scenario: Catalogue holds jobs

- **WHEN** the catalogue holds at least one job
- **THEN** `freehire_catalogue_newest_job_timestamp_seconds` MUST report the newest job's creation time as seconds since the Unix epoch

#### Scenario: Catalogue is empty

- **WHEN** the catalogue holds no jobs
- **THEN** the metric MUST be omitted rather than published as `0`

Zero would be read as 1970 and therefore as an infinitely stale catalogue. An empty
catalogue is a fresh-install state, not an incident, so no value is the honest answer.

### Requirement: Metrics are published through the textfile collector

The metrics SHALL be published by writing a Prometheus text-format file into the
directory named by `PROM_TEXTFILE_DIR`, matching the mechanism the existing per-run
worker metrics already use. When `PROM_TEXTFILE_DIR` is unset, the worker SHALL collect
nothing and exit successfully, so an unconfigured deployment is unaffected.

The file SHALL be written atomically, so a collector reading the directory concurrently
observes either the previous complete file or the new complete file, never a partial one.

#### Scenario: Collector directory is configured

- **WHEN** the worker runs with `PROM_TEXTFILE_DIR` set
- **THEN** it MUST write a single Prometheus text-format file into that directory
- **AND** each metric family MUST carry its `# HELP` and `# TYPE` lines

#### Scenario: Collector directory is unset

- **WHEN** the worker runs with `PROM_TEXTFILE_DIR` empty or unset
- **THEN** it MUST NOT query the database, MUST NOT write any file, and MUST exit zero

#### Scenario: Write is interrupted

- **WHEN** the worker is killed partway through writing the file
- **THEN** the previously published file MUST remain intact and parseable

### Requirement: Collection failure is visible and contained

A failure to collect or publish the metrics SHALL cause the worker to exit non-zero, so
its own run-outcome metric records the failure. The worker SHALL take no database locks
and SHALL perform no writes, so a slow or hung collection cannot block ingest, search
drain, or reindex.

#### Scenario: A query fails

- **WHEN** one of the aggregate queries returns an error
- **THEN** the worker MUST log the error and exit non-zero

#### Scenario: Concurrent pipeline work

- **WHEN** the worker runs while ingest, search drain, or reindex are working
- **THEN** it MUST NOT acquire locks that those workers contend on, and MUST NOT modify any row
