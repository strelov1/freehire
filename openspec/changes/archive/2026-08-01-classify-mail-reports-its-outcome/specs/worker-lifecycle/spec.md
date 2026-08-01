## MODIFIED Requirements

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
