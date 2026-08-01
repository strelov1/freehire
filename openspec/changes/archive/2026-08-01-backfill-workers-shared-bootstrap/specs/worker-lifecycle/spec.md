## MODIFIED Requirements

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
