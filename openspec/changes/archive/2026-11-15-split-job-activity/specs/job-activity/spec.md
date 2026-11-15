## ADDED Requirements

### Requirement: Separate observation persistence
Each stored posting SHALL have one activity row containing its last observation, hydration timestamp and liveness strikes. Ordinary observations SHALL preserve the main posting tuple and content-change timestamp.

#### Scenario: Repeated unchanged observation
- **WHEN** an open posting is observed again without a content or lifecycle transition
- **THEN** its activity timestamp advances
- **AND** the `jobs` tuple's xmin, xmax and ctid remain unchanged

### Requirement: Preserve public and internal reads
All consumers SHALL read observation state from the activity relation after the reader rollout. The public JSON schema SHALL remain unchanged.

#### Scenario: Stale legacy fields after writer cutover
- **GIVEN** the legacy observation values differ from current activity
- **WHEN** detail, list, search projection, hydration selection or a lifecycle predicate reads the posting
- **THEN** it uses current activity rather than legacy fields

### Requirement: Preserve concurrent lifecycle and content decisions
A stale probe or sweep SHALL NOT close a freshly observed posting. A cheap refresh that raced a lifecycle or cheap-match-key change SHALL retry the existing full persistence path after releasing its activity lock.

#### Scenario: Observation precedes stale closure
- **WHEN** an observation commits while a closer is waiting
- **THEN** the closer rechecks the locked current activity and skips the fresh posting

#### Scenario: Closure precedes waiting refresh
- **WHEN** closure commits after a cheap refresh selected its candidate
- **THEN** that refresh is fenced from silently stamping a closed posting
- **AND** the full path applies the existing reopen or deferred-reopen rules

#### Scenario: Concurrent content change
- **WHEN** the cheap match key changes after selection but before a cheap refresh acquires its activity lock
- **THEN** the refresh retries full persistence with the original input, preserving current conflict semantics

### Requirement: Preserve atomic transitions and queues
Deadlines, strikes, reopening, withholding and their queue side effects SHALL retain their existing transactional semantics. Transactions that lock both relations SHALL take posting locks before activity locks.

#### Scenario: Reopening
- **WHEN** a closed posting is legitimately reopened
- **THEN** the observation and strike reset are consistent with the reopened state
- **AND** the existing search restoration is queued atomically

### Requirement: Compatible deployment and bounded backfill
Schema expansion SHALL keep old binaries operational. The backfill SHALL be restartable and bounded in transaction size, and SHALL finish before activity readers are deployed. Activity-only writers SHALL start only after pre-expansion readers are gone. Legacy columns SHALL be dropped only after every running query has stopped naming them.

#### Scenario: Legacy writer during backfill
- **WHEN** a legacy source observation commits during the backfill
- **THEN** the activity row reflects that observation and backfill does not overwrite it

#### Scenario: Contract migration
- **WHEN** legacy columns are removed
- **THEN** the currently running writer-release binaries continue to execute all queries successfully
