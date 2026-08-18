# worker-lifecycle Specification

## Purpose
TBD - created by archiving change reliable-worker-bootstrap. Update Purpose after archive.
## Requirements
### Requirement: Shared worker bootstrap

Every standalone run-once-and-exit worker SHALL obtain its runtime dependencies
(loaded config, an open database pool, and a root context) from one shared
bootstrap helper rather than re-implementing the setup inline. The helper SHALL
open the pgx pool and return a cleanup function that closes it.

The rule binds by behaviour, not by naming: any binary under `cmd/` that opens a
database pool is a worker for this purpose. `cmd/server` is the single exemption —
it is a long-lived daemon rather than a run-once process and owns its own startup
and shutdown. Binaries that never open a pool (code generators, harvest tools that
write files) are out of scope.

Compliance SHALL be enforced mechanically rather than by convention, and the set of
exempt binaries SHALL be declared in that enforcement, so a new worker that
hand-rolls its setup fails the suite instead of drifting silently.

#### Scenario: Bootstrap provides pool and cleanup

- **WHEN** a worker calls the shared bootstrap helper with a valid database URL
- **THEN** it receives a usable database pool, a root context, and a cleanup
  function that closes the pool when invoked

#### Scenario: Bootstrap fails fast on an unreachable database

- **WHEN** the bootstrap helper cannot connect to the database
- **THEN** it returns an error (no usable pool), and the worker terminates with a
  non-zero exit code rather than proceeding

#### Scenario: A database-touching binary bypasses the bootstrap

- **WHEN** a binary under `cmd/` opens a database pool without going through the
  shared bootstrap, and is not in the declared exemption list
- **THEN** the test suite fails, naming that binary

#### Scenario: A worker exits without running its deferred cleanup

- **WHEN** a worker's run ends in failure
- **THEN** it returns a non-zero code from its run function rather than calling
  `os.Exit` or `log.Fatal` directly, so the deferred pool close and any buffered
  telemetry flush still run

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

A queue drain MUST count a failure of the bookkeeping call that records an item as failed
toward the run's failure total (so the run reports a non-zero outcome), and SHALL log the error
cause — so an operator can diagnose why the bookkeeping write failed instead of seeing only an
opaque failure tally.

This binds every queue drain, not only the enrichment one. A drain whose bookkeeping write fails
does not know whether the item dead-lettered, and it MUST NOT guess: the item counts as failed
and its dead-letter state is left unrecorded, because the entry is then governed by its lease
expiry rather than by a stamp the run never wrote.

#### Scenario: A failed bookkeeping write is counted and logged

- **WHEN** the runner's call to mark a job as failed itself returns an error
- **THEN** the failure is counted toward the run's failure total (the run's
  outcome is non-zero) and the error cause is written to the log

#### Scenario: A drain does not guess a dead-letter it could not record

- **WHEN** a queue drain's call to record a failed attempt itself fails, and that call is what
  would have reported whether the item dead-lettered
- **THEN** the run counts the item as failed and does NOT count it as dead-lettered

### Requirement: External pause switch refuses a worker run

A run-once worker SHALL consult an external pause switch before performing any work, and
SHALL exit successfully without working while that switch is held. The check SHALL occur in
the shared entry wrapper, before the worker's run function is called, so that no worker can
opt out of it and a refused run costs neither a database pool nor an error-reporting
handshake.

The switch SHALL be expressed as the presence of either of two Redis keys: one naming the
whole fleet, and one naming a single worker binary. Presence alone is the signal; the stored
value carries no meaning. A key naming a single worker SHALL match on the binary's name only,
not on any per-invocation argument, so that a fleet of same-binary runs (the ~140 ingest
boards) is held or released as one.

A refused run SHALL exit zero. A pause is an operator's decision, not a failure, and must not
be reported as one.

An environment variable SHALL bypass the switch entirely, so that an operator can run one
worker by hand against an otherwise-held fleet. Because systemd timer units do not carry that
variable, the bypass admits only what a human started deliberately.

The check SHALL fail open. If the switch cannot be read — an unreachable server, a malformed
URL, or a response slower than a short fixed timeout — the worker SHALL log the failure and
proceed with its normal run. A facility for shedding load must never itself become the reason
the catalogue stops updating.

#### Scenario: No switch is held

- **WHEN** neither the fleet-wide key nor the worker's own key is present
- **THEN** the worker runs normally

#### Scenario: The fleet-wide switch holds every worker

- **WHEN** the fleet-wide key is present
- **THEN** the worker exits zero without calling its run function

#### Scenario: A per-worker switch holds only that binary

- **WHEN** a key naming the `ingest` binary is present
- **THEN** an `ingest` run is refused, and a `search-drain` run proceeds

#### Scenario: A per-worker switch ignores invocation arguments

- **WHEN** a key naming the `ingest` binary is present
- **THEN** every `ingest` invocation is refused regardless of which board file it was given

#### Scenario: The override admits a hand-started run

- **WHEN** the fleet-wide key is present and the bypass variable is set in the environment
- **THEN** the worker runs normally

#### Scenario: An unreachable switch does not stop the fleet

- **WHEN** the Redis server cannot be reached
- **THEN** the worker logs the failure and runs normally

#### Scenario: A slow switch does not stall the fleet

- **WHEN** reading the switch takes longer than the fixed timeout
- **THEN** the worker stops waiting, logs the failure, and runs normally

### Requirement: A refused run is distinguishable from a completed one

The metrics a worker publishes SHALL distinguish a run refused by the pause switch from a run
that executed and succeeded. A refused run SHALL publish a dedicated paused gauge and SHALL
NOT refresh the worker's last-run timestamp, duration, or success series.

This is deliberate: leaving the last-run series to age means an existing staleness alert still
fires for a switch nobody lifted, while the paused gauge beside it identifies the silence as
intentional. Stamping a refused run as a success would make a held switch indistinguishable
from a healthy fleet — the failure mode already observed on this host, where skipped reindex
cycles recorded as successes kept a dashboard green while the search index went stale.

A completed run SHALL publish the paused gauge as zero alongside its last-run series, so the
gauge is a live signal rather than one that merely stops being written.

Both the paused gauge and the existing last-run series SHALL be published to the worker's
single existing metrics file, so that no worker owns two textfile names and the last writer
cannot silently overwrite the other's payload.

#### Scenario: A refused run publishes the paused gauge alone

- **WHEN** a worker's run is refused by the pause switch
- **THEN** its metrics file contains the paused gauge set to one, and contains no last-run
  timestamp, duration, or success series

#### Scenario: A completed run clears the paused gauge

- **WHEN** a worker completes a run normally
- **THEN** its metrics file contains the last-run series and the paused gauge set to zero

#### Scenario: Both states share one metrics file

- **WHEN** a worker publishes either a refused or a completed run
- **THEN** the output is written to the same single filename that worker already owns

