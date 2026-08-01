# corruption-resilience Specification

## Purpose
TBD - created by archiving change resilient-toast-corruption. Update Purpose after archive.
## Requirements
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

### Requirement: The derive backfill completes despite corrupted rows

The whole-catalogue derive backfill SHALL read jobs through the resilient full-scan
helper, so a row that cannot be read due to data corruption is skipped and logged rather
than ending the run. Every readable row after the corrupted one SHALL still be re-derived
in the same pass.

Because the backfill records no resume point, an aborting scan is not merely a failed run:
it re-fails at the same id on every subsequent run, so every deterministic column past that
row stays stale indefinitely. Skipping is what makes the pass finishable at all.

The scan SHALL treat a keyset cursor that did not advance as its exhaustion signal, and
SHALL NOT treat a page shorter than the batch size as the end of the table. The degrade
path returns a short page whenever it skips a damaged row, so the shorter-than-batch test
would end the scan at the first corrupted row and report a complete pass.

#### Scenario: A corrupted row does not end the backfill

- **WHEN** the derive backfill's scan meets a row that fails to read with a data-corruption
  error
- **THEN** that row is skipped and logged, and the rows after it are still scanned and
  re-derived in the same run

#### Scenario: A short page from the degrade path does not end the scan

- **WHEN** a page returns fewer rows than the batch size because a corrupted row was
  skipped
- **THEN** the scan continues from the advanced cursor rather than treating the short page
  as the end of the table

#### Scenario: A non-corruption read failure still ends the run

- **WHEN** the scan's read fails with an error that is not data corruption
- **THEN** the run fails with that error, exactly as before — the resilience is narrow to
  corruption and hides nothing else

