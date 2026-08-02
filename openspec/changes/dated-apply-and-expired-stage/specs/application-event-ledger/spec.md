## ADDED Requirements

### Requirement: Correcting an application's date corrects its applied event

When an application's `applied_at` is corrected, the system SHALL move the `occurred_at` of its
`applied` event to the same instant, in the same transaction as the correction.

The event states when the person applied, not when the system was told, and every aggregate
that counts applications or measures how long an employer took to answer reads `occurred_at`.
Correcting one record and not the other leaves the card reporting one month and the statistics
another — two accounts of a single transition, which is the divergence this ledger exists to
prevent.

The correction SHALL NOT write a second `applied` event. One application produced one
application event; restating its date is a repair of that event, not a new fact, and a second
row would inflate the denominator of every response-rate aggregate.

#### Scenario: The column and the ledger agree after a correction

- **WHEN** an application recorded today is corrected to a date last month
- **THEN** its `applied` event carries that same date
- **AND** the pipeline snapshot and the per-company response rate count it under that date

#### Scenario: A correction adds no event

- **WHEN** an application's date is corrected
- **THEN** it still has exactly one `applied` event

#### Scenario: Recording when we learned it is untouched

- **WHEN** an application's date is corrected
- **THEN** the event's `recorded_at` still reports when the row was first written, so the
  distinction between when it happened and when it was learned survives the repair
