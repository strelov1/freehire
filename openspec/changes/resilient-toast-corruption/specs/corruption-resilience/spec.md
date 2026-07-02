## ADDED Requirements

### Requirement: Full-scan reads survive corrupted rows

A full-catalogue keyset scan SHALL NOT abort when an individual row cannot be read due to Postgres data corruption. When a batch read fails with SQLSTATE `XX001` (data corruption, e.g. a missing TOAST chunk), the scan SHALL degrade to reading the affected batch row-by-row, skip only the rows that still fail with `XX001`, log each skipped row's id, advance the keyset past them, and continue the scan to completion.

Errors other than `XX001` SHALL propagate unchanged (the resilience path is narrow to data corruption only, so unrelated failures still surface).

#### Scenario: healthy batch reads normally

- **WHEN** a batch read succeeds
- **THEN** the helper returns the batch rows and the last id, with no skipped rows and no extra queries

#### Scenario: batch contains one corrupted row

- **WHEN** a batch read fails with SQLSTATE `XX001`
- **THEN** the helper lists the batch's ids (a projection that does not detoast), fetches each row individually, returns every readable row, records the corrupted row's id as skipped, logs it, and advances the keyset past the batch

#### Scenario: non-corruption error is not swallowed

- **WHEN** a batch read fails with an error whose SQLSTATE is not `XX001`
- **THEN** the helper returns that error unchanged and does not enter the row-by-row degrade path

### Requirement: Reindex completes despite corrupted rows

The `reindex` worker SHALL read jobs through the resilient full-scan helper so that a corrupted row does not prevent the index rebuild from reaching the swap. Skipped rows SHALL be counted and reported in the run's log summary; a corrupted row is simply absent from the rebuilt index (it is unreadable) rather than aborting the rebuild.

#### Scenario: reindex with a corrupted row still swaps in

- **WHEN** a full reindex encounters a corrupted row mid-scan
- **THEN** the corrupted row is skipped and logged, the remaining jobs are indexed, and the fresh index is swapped in

### Requirement: Enrichment fast-fails on corrupted rows

The `enrich` worker SHALL classify a per-job read that fails with SQLSTATE `XX001` as a non-retryable (corrupted) failure and dead-letter the outbox entry immediately, rather than consuming its retry budget on an unreadable row.

#### Scenario: enrich claims a corrupted job

- **WHEN** enrichment reads a claimed job and the read fails with SQLSTATE `XX001`
- **THEN** the entry is marked dead-lettered without retry and the worker continues draining other entries

### Requirement: Graceful database shutdown

The Postgres container SHALL be given enough shutdown grace for a clean fast-shutdown to complete before the container runtime sends SIGKILL, so the database is not killed mid-write (a corruption trigger).

#### Scenario: stopping the DB container

- **WHEN** the Postgres container is stopped (deploy, restart, or manual `docker stop`)
- **THEN** Postgres receives its stop signal and completes a clean shutdown within the configured grace period before any SIGKILL

### Requirement: Corruption detection and repair

Operators SHALL be able to detect corrupted rows across the catalogue and repair them. Detection SHALL enumerate the ids of rows that fail to read (`XX001`). Repair SHALL make a corrupted row readable again by clearing the corrupted field, accepting that its value is re-populated on the row's next ingest or enrich refresh.

#### Scenario: scan reports corrupted ids

- **WHEN** the corruption scan runs over the `jobs` table
- **THEN** it reports the ids of every row that cannot be fully read

#### Scenario: repair restores readability

- **WHEN** a corrupted row is repaired
- **THEN** the row can be read in full afterwards and is eligible for indexing and enrichment again
