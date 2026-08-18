## ADDED Requirements

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
