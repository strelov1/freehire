## ADDED Requirements

### Requirement: The calendar shows what is arranged as well as what happened

The calendar SHALL show the caller's scheduled interviews alongside the events recorded
in the ledger, and SHALL present the two so they cannot be mistaken for each other: a
recorded event is something that happened, a scheduled meeting is something arranged that
may still move or be called off.

A scheduled meeting SHALL be placed on the day it is due in the reader's own timezone,
by the same rule every other mark follows. Its day panel entry SHALL carry the time, what
the meeting is called, and the way to join it where the invitation gave one.

A cancelled meeting SHALL be shown as cancelled rather than removed. A candidate who
remembers an interview on Thursday and finds an empty Thursday cannot tell a cancellation
from a fault in the calendar.

#### Scenario: A scheduled interview appears on its own day

- **WHEN** the caller has an interview arranged for a day inside the rendered month
- **THEN** that day carries a mark for it, distinct from the marks for recorded events

#### Scenario: The day panel gives what is needed to attend

- **WHEN** the caller opens the day of a scheduled interview
- **THEN** the panel shows its time, its title, the application it belongs to, and the
  joining link when there is one

#### Scenario: A cancellation is visible

- **WHEN** an interview the calendar previously showed is cancelled
- **THEN** it is still shown for that day, marked as cancelled

#### Scenario: A month with only future interviews is not empty

- **WHEN** a month holds no recorded events but does hold a scheduled interview
- **THEN** the view shows the interview rather than the empty-month message
