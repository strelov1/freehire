## ADDED Requirements

### Requirement: Mock hosts listing
An active member SHALL be able to list the members they could book a mock with now: active
members other than themselves with saved, unpaused settings, no block with the viewer in
either direction, and at least one open slot between 12 hours and 30 days from now. Each
entry SHALL carry the member's handle, the card summary other buddy lists show, their
topics, their soonest open slot in the viewer's zone, and the count of completed mocks and
the count and average of revealed reviews under the member-page rules. An optional topic
SHALL narrow the listing to hosts offering it, and an unknown topic SHALL be refused with a
validation error. The listing SHALL be ordered by soonest open slot, then by handle, and
SHALL carry no meeting link. A non-member SHALL be refused.

#### Scenario: Members who cannot be booked are absent
- **WHEN** the viewer lists hosts and other members are paused, have no availability, have
  left, or block or are blocked by the viewer
- **THEN** none of them is listed, and neither is the viewer

#### Scenario: Topic filter
- **WHEN** the viewer lists hosts for `behavioral`
- **THEN** only hosts offering `behavioral` are listed

#### Scenario: Unknown topic
- **WHEN** the viewer lists hosts for a topic outside `coding`, `system_design`,
  `behavioral`
- **THEN** the system responds with a validation error

#### Scenario: Soonest first
- **WHEN** two hosts have open slots at different times
- **THEN** the host with the earlier slot is listed first, and hosts with the same next slot
  are ordered by handle

#### Scenario: A block withdraws the rating
- **WHEN** a host's only revealed review was written by a member who has since blocked the
  host
- **THEN** the host's entry shows no rating and still counts the completed mock
