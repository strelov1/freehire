# worker-lifecycle Specification

## Purpose
TBD - created by archiving change reliable-worker-bootstrap. Update Purpose after archive.
## Requirements
### Requirement: Shared worker bootstrap

Every standalone run-once-and-exit worker SHALL obtain its runtime dependencies
(loaded config, an open database pool, and a root context) from one shared
bootstrap helper rather than re-implementing the setup inline. The helper SHALL
open the pgx pool and return a cleanup function that closes it.

#### Scenario: Bootstrap provides pool and cleanup

- **WHEN** a worker calls the shared bootstrap helper with a valid database URL
- **THEN** it receives a usable database pool, a root context, and a cleanup
  function that closes the pool when invoked

#### Scenario: Bootstrap fails fast on an unreachable database

- **WHEN** the bootstrap helper cannot connect to the database
- **THEN** it returns an error (no usable pool), and the worker terminates with a
  non-zero exit code rather than proceeding

### Requirement: Graceful cancellation on termination signals

The root context returned by the shared bootstrap SHALL be cancelled when the
process receives `SIGINT` or `SIGTERM`, so in-flight work observes cancellation
and unwinds instead of being hard-killed. Workers SHALL propagate this context
into their run/sweep calls.

#### Scenario: SIGTERM cancels the worker context

- **WHEN** the process receives `SIGTERM` during a run
- **THEN** the bootstrap context is cancelled and the in-flight database/run
  operations observe the cancellation through the propagated context

#### Scenario: Signal handler is released after the run

- **WHEN** the worker finishes and invokes its cleanup function
- **THEN** the signal notification is stopped (the process no longer intercepts
  `SIGINT`/`SIGTERM` for the cancelled context)

### Requirement: Run outcome reported through exit code

A worker process SHALL exit with a non-zero code when its run completes with one
or more per-item failures or dead-lettered items, and SHALL exit `0` when the run
completes with zero failures. Per-item failure isolation is
preserved — a single failing item MUST NOT abort the remaining items — but the
aggregate failure MUST be reflected in the exit code so cron can alert.

#### Scenario: Clean run exits zero

- **WHEN** a worker run completes with no failures and no dead-letters
- **THEN** the process exits with code `0`

#### Scenario: Run with failures exits non-zero

- **WHEN** a worker run completes but its run stats report at least one failure
  or dead-lettered item
- **THEN** the process exits with a non-zero code

#### Scenario: One bad item does not abort the run

- **WHEN** a single item in a run fails (e.g. one unreachable board or one
  unparseable post)
- **THEN** the remaining items are still processed, and the run still exits
  non-zero afterward to signal the partial failure

### Requirement: Bookkeeping failures are logged and counted

The enrichment drain MUST count a failure of the queue bookkeeping call that
records a job as failed toward the run's failure total (so the run reports a
non-zero outcome), and SHALL log the error cause — so an operator can diagnose
why the bookkeeping write failed instead of seeing only an opaque failure tally.

#### Scenario: A failed bookkeeping write is counted and logged

- **WHEN** the runner's call to mark a job as failed itself returns an error
- **THEN** the failure is counted toward the run's failure total (the run's
  outcome is non-zero) and the error cause is written to the log

