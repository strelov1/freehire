## ADDED Requirements

### Requirement: The set of scheduled providers is derived from the board catalog

The system SHALL derive the set of providers eligible for scheduling from the `boards`
table — the distinct `provider` values holding at least one row with `status` in
(`pending`, `active`) — and never from a directory listing, a unit-file name, or any other
second spelling of the provider key. A provider whose every board is `retired` or
`rejected` SHALL stop being scheduled without any operator action.

This is the requirement that closes the class of defect this change exists for: the
provider key that selects boards and the provider key that schedules the crawl SHALL be
the same string read from the same row.

#### Scenario: A provider with live boards is eligible

- **WHEN** the catalog holds at least one `pending` or `active` board for provider `ashby`
- **THEN** `ashby` is in the scheduler's eligible set

#### Scenario: A provider whose boards are all retired stops being scheduled

- **WHEN** every board of provider `careerspage` is moved to `retired`
- **THEN** the scheduler no longer schedules `careerspage`, and no operator step is needed
  to stop it

#### Scenario: A newly activated board's provider is scheduled without operator action

- **WHEN** a crowdsourced board for a provider with no previous live board is activated by
  its first successful crawl
- **THEN** that provider becomes eligible on the scheduler's next tick

### Requirement: An empty roster is refused, not obeyed

The system SHALL refuse a tick that finds NO eligible provider, and SHALL leave run state
untouched when it does. Reconciliation deletes the state of every provider absent from the
list it is given, so acting on an empty roster would erase the whole fleet's schedule —
including the stagger that keeps providers from crawling on the same minute — on the
strength of one bad read.

A roster that is non-empty but currently has nothing SCHEDULABLE SHALL be accepted
normally: that is the expected state on the first day of a rollout, not a failure.

There SHALL be no threshold above zero. A legitimately smaller catalogue must still be
schedulable, and a "fewer than N looks wrong" floor would block one while catching nothing
that zero does not.

#### Scenario: No eligible provider fails the tick

- **WHEN** the roster read returns no provider at all
- **THEN** the tick reports an error and reconciles nothing

#### Scenario: A roster with nothing schedulable is not an error

- **WHEN** every eligible provider is disabled or not yet handed to the scheduler
- **THEN** the tick succeeds, launches nothing, and reports each provider's reason

### Requirement: A provider absent from the configuration table is scheduled on defaults

The system SHALL treat the configuration table as a set of OVERRIDES, not as the roster.
An eligible provider with no `ingest_schedule` row SHALL be scheduled on documented
defaults (one shard, hourly, the default per-run timeout) rather than being skipped.

A provider SHALL be excluded from scheduling only by an `ingest_schedule` row with
`enabled` false, and such a row SHALL carry a non-empty `disabled_reason`. Absence of
configuration and a deliberate decision not to crawl SHALL NOT be representable by the
same state.

#### Scenario: An unconfigured provider still runs

- **WHEN** a provider is eligible and has no `ingest_schedule` row
- **THEN** it is scheduled hourly, unsharded, on the default timeout

#### Scenario: Disabling a provider requires a stated reason

- **WHEN** a write sets `enabled` false without a `disabled_reason`
- **THEN** the write is refused

#### Scenario: A disabled provider is not launched but stays visible

- **WHEN** a provider's row has `enabled` false with a reason
- **THEN** no run is launched for it, and the scheduler's report lists it as disabled with
  that reason

### Requirement: A sharded provider schedules each shard independently

The system SHALL hold run state per `(provider, shard)`. A provider configured with `n`
shards SHALL have `n` independently due rows, each launching `cmd/ingest <provider>
--shard=i/n`. Changing a provider's shard count SHALL reconcile its run-state rows —
adding the new shards and removing those beyond the new count — without disturbing the
shards that remain.

#### Scenario: Each shard becomes due on its own schedule

- **WHEN** a provider is configured with 4 shards
- **THEN** four run-state rows exist, and each becomes due independently

#### Scenario: Reducing the shard count removes the surplus rows

- **WHEN** a provider configured with 24 shards is reconfigured to 12
- **THEN** shards 13-24 are removed from run state and shards 1-12 keep their due times

#### Scenario: A shard's run names its own slice

- **WHEN** shard 3 of a 6-shard provider is launched
- **THEN** the launched command carries `--shard=3/6`

### Requirement: A due run is claimed exactly once

The system SHALL claim a due run under `SELECT ... FOR UPDATE SKIP LOCKED` before
launching it, so two concurrent scheduler invocations cannot launch the same
`(provider, shard)` twice. Claiming SHALL record the claim time, and the next due time
SHALL be advanced by the provider's cadence at claim time rather than at completion, so a
long run does not push its own successor arbitrarily far into the future.

#### Scenario: Two concurrent schedulers do not double-launch

- **WHEN** two scheduler invocations overlap and both see the same due row
- **THEN** exactly one claims and launches it, and the other skips it without error

#### Scenario: A claimed run is not re-claimed while it is running

- **WHEN** a run has been claimed and has neither finished nor exceeded its reclaim window
- **THEN** subsequent ticks do not launch it again

### Requirement: A stuck run is reclaimed after its own timeout plus a grace window

The system SHALL treat a claim older than that provider's configured per-run timeout plus
a grace window as dead, and SHALL make the row claimable again. A scheduler that crashed
between claiming and launching, and a run systemd killed at its timeout, SHALL both
recover without operator action.

#### Scenario: A claim outliving its timeout is reclaimed

- **WHEN** a run's claim is older than its provider's timeout plus the grace window and no
  finish was recorded
- **THEN** the next tick may claim and launch it again

#### Scenario: A run still inside its budget is left alone

- **WHEN** a run's claim is younger than its provider's timeout
- **THEN** it is not reclaimed, however far past its cadence it now is

### Requirement: Fleet concurrency is bounded by the scheduler

The system SHALL bound how many ingest runs it launches concurrently, counting the runs it
believes to be in flight, and SHALL launch at most that many minus those already running on
any one tick. Counting and claiming SHALL be serialised against other instances of the
scheduler, so two overlapping invocations cannot each read the same free capacity and each
consume all of it. A tick that can launch nothing because the fleet is saturated SHALL report
that it skipped rather than exiting silently, and SHALL leave the skipped rows due so the
next tick picks them up.

#### Scenario: A saturated fleet launches nothing and says so

- **WHEN** the number of in-flight runs already equals the cap
- **THEN** the tick launches nothing, logs that it was saturated, and leaves every due row
  claimable

#### Scenario: A partially loaded fleet launches only the free capacity

- **WHEN** the cap is 10 and 7 runs are in flight
- **THEN** at most 3 runs are launched on that tick

#### Scenario: A second concurrent scheduler does not double the ceiling

- **WHEN** a second scheduler invocation starts while one is already deciding
- **THEN** it exits without claiming, rather than measuring the same free capacity again

### Requirement: A run is launched as a transient unit carrying its own timeout

The system SHALL launch each claimed run as a transient systemd unit whose start timeout is
the provider's configured per-run timeout, so a provider needing a longer budget does not
force every other provider onto the same one. The launched unit SHALL run as the
unprivileged service account, and SHALL outlive the scheduler invocation that started it.

#### Scenario: The per-provider timeout reaches the launched unit

- **WHEN** a provider configured with a 4500-second timeout is launched
- **THEN** the transient unit carries that timeout, not the default

#### Scenario: The scheduler exits without killing its runs

- **WHEN** the scheduler has launched runs and exits
- **THEN** the launched runs continue

### Requirement: A finished run is noticed by the scheduler, not reported by itself

A launched run SHALL NOT be relied upon to announce its own completion — it is an ordinary
`cmd/ingest` process that knows nothing about scheduling. The scheduler SHALL therefore ask
the service manager about every claimed run, release the claims of those that have ended,
and count only the ones still executing.

This SHALL happen BEFORE free capacity is measured, so a tick that has just released
capacity may use it. It SHALL happen in shadow mode as well as in apply mode.

A run whose status cannot be read SHALL be counted as still executing and reported. Over-
counting costs one slot for one tick; under-counting launches a second copy of a crawl that
is already running.

#### Scenario: A run whose unit has ended releases its claim

- **WHEN** a claimed run's unit no longer exists
- **THEN** its claim is released, its outcome recorded, and it stops counting against the
  concurrency cap

#### Scenario: A run still executing keeps its claim

- **WHEN** a claimed run's unit is still active
- **THEN** its claim is untouched and it counts against the cap

#### Scenario: Capacity freed this tick is usable this tick

- **WHEN** nine of ten in-flight runs are found to have ended
- **THEN** the same tick treats nine slots as free rather than reporting saturation

#### Scenario: An unreadable run status does not free a slot

- **WHEN** the service manager cannot be asked about a claimed run
- **THEN** the run is counted as executing and the failure is reported

### Requirement: A run's outcome is recorded against its schedule row

The system SHALL record, per `(provider, shard)`, when the last run started, when it
finished, and how it ended, so "this provider has not completed a run since" is answerable
in SQL without reading the journal.

#### Scenario: A finished run records its outcome

- **WHEN** a launched run completes
- **THEN** its finish time and exit status are recorded against its row

#### Scenario: Never-completed runs are reportable

- **WHEN** an operator asks which providers have not finished a run within their cadence
- **THEN** the answer is available from run state alone

### Requirement: The scheduler ships in shadow mode

The system SHALL support a mode in which the scheduler performs every step except the
launch — resolving eligibility, applying configuration, choosing what is due, and reporting
each decision — while launching nothing and advancing no due time. This mode SHALL be the
default, so the first deployment cannot disturb a fleet still driven by the existing
timers, and enabling launches SHALL be an explicit operator act.

#### Scenario: Shadow mode reports without launching

- **WHEN** the scheduler runs with launching not enabled
- **THEN** it reports what it would have launched, launches nothing, and leaves run state
  unchanged

#### Scenario: Launching is off unless explicitly enabled

- **WHEN** the scheduler is deployed with no launch setting configured
- **THEN** it runs in shadow mode

### Requirement: A curator reads and edits the schedule through a dedicated worker

The system SHALL provide a command that reports the whole schedule — every eligible
provider, its effective cadence, shard count and timeout, whether the value is a default or
an override, and its last run outcome — and that writes only under an explicit apply flag,
mirroring `cmd/add-board`. Every override SHALL be able to carry a free-text note recording
why the value is what it is.

#### Scenario: The command reports by default

- **WHEN** the command is run with no apply flag
- **THEN** it prints the effective schedule and writes nothing

#### Scenario: An override is written only under apply

- **WHEN** the command is run with a new cadence and the apply flag
- **THEN** the override is persisted

#### Scenario: The report distinguishes a default from an override

- **WHEN** a provider has no configuration row and another has one
- **THEN** the report marks the first as running on defaults and the second as overridden
