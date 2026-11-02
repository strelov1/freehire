## ADDED Requirements

### Requirement: Search removals respect reopening
A removal SHALL skip currently open jobs while still deleting closed or missing jobs. Acknowledging a pending removal SHALL atomically queue currently open public canonical jobs for indexing. A hydrating-source liveness touch that reopens a job SHALL atomically enqueue its return to search; an ordinary open-to-open touch SHALL enqueue nothing.

#### Scenario: Job reopens before deletion
- **WHEN** a job with a pending removal is open before the deletion wave
- **THEN** the document is not submitted for removal
- **AND** acknowledging the stale removal queues its current content for indexing

#### Scenario: Job reopens during deletion
- **WHEN** a closed job reopens while the index deletion is in flight
- **THEN** acknowledgment queues the open job for indexing before discarding the old removal

#### Scenario: Job reopens after deletion
- **WHEN** a hydrating-source touch reopens a previously removed posting
- **THEN** it is queued for indexing without changing its stored content

#### Scenario: Job row has been hard deleted
- **WHEN** a pending removal names a missing job row
- **THEN** its document is removed and the removal is acknowledged without an indexing enqueue

#### Scenario: Touch waits for a concurrent close
- **WHEN** a liveness touch starts while a closing transaction holds the job row lock
- **THEN** transition detection uses the closed row state after that transaction commits
- **AND** reopening queues the job for indexing
