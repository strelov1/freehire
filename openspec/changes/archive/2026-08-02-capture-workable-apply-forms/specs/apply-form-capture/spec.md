## ADDED Requirements

### Requirement: Workable's form is captured through the queue

The system SHALL capture the application form Workable publishes for a posting,
through the same per-posting queue Greenhouse and Ashby use. A Workable posting SHALL
be queued by the ingest write path and drained by the capture worker, gated and
retried exactly as those two are.

The fetch SHALL be addressed by the platform's own posting shortcode, which the
stored external id already carries — no board, account or lookup is required.

#### Scenario: A Workable posting is queued and captured

- **WHEN** ingest writes a Workable posting that has no stored form
- **THEN** a capture is queued for it, and the worker fetches and stores its form

#### Scenario: The fetch needs only the shortcode

- **WHEN** the worker captures a Workable posting
- **THEN** it addresses the platform using the posting id half of the stored external
  id alone

### Requirement: Workable's option pair is read in its own order

Workable names an enumerated answer's two halves the opposite way round from every
other captured platform: the identifier is under `name` and the text a candidate
reads is under `value`.

The mapper SHALL store the human text as the option's label and the identifier as the
option's value, so that a captured Workable option means the same thing as a captured
option from any other platform.

#### Scenario: An enumerated answer keeps its meaning

- **WHEN** Workable describes an answer as `{"name": "6166574", "value": "I actively
  attend AI industry events"}`
- **THEN** the captured option reads "I actively attend AI industry events" and
  carries `6166574` as the value to submit

### Requirement: A Workable field group is captured as one control

Workable describes the candidate's education and work history as repeatable groups,
each carrying its own nested fields. The mapper SHALL capture such a group as a single
control named by the group, and SHALL NOT capture its nested fields individually — the
fact worth recording is that the application asks for an education history, not that
an education entry has five parts.

#### Scenario: A group does not become five controls

- **WHEN** Workable describes an Education group containing school, field of study,
  degree and two dates
- **THEN** the capture holds one control for the group and none for its parts
