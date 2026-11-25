## ADDED Requirements

### Requirement: A meeting link is required only without a connected calendar

A mentor profile SHALL require a meeting link only when its owner has not connected
Google Calendar for write access. A mentor who has connected it MAY submit or update
their profile with an empty meeting link, because a real one is minted per booking
instead.

#### Scenario: A meeting link is still required without a connected calendar

- **WHEN** a mentor with no connected calendar submits a profile with an empty meeting
  link
- **THEN** the submission is refused, exactly as it always has been

#### Scenario: A connected mentor may omit the meeting link

- **WHEN** a mentor who has connected Google Calendar for write access submits or
  updates their profile with an empty meeting link
- **THEN** the submission succeeds

#### Scenario: A connected mentor may still set one

- **WHEN** a mentor who has connected Google Calendar for write access submits a
  non-empty meeting link
- **THEN** it is stored exactly as any other profile field
- **AND** no booking of theirs ever reads it while the connection stands, whether or
  not a given booking's own link generation succeeds
