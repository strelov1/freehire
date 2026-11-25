## ADDED Requirements

### Requirement: A form may be read from a rendered page, not only a JSON API

The system SHALL be able to capture an application form from a platform that
publishes it as a rendered page rather than as structured data, using the same queue,
worker, retry and gone-handling as the platforms that publish JSON.

#### Scenario: A form is captured from markup

- **WHEN** the worker captures a posting whose platform publishes its form as a page
- **THEN** the form is stored with the same shape a JSON platform's would be

### Requirement: Lever's form is captured through the queue

The system SHALL capture the application form Lever publishes for a posting. A Lever
posting SHALL be queued by the ingest write path and drained by the capture worker,
gated and retried exactly as the other captured providers are.

The fetch SHALL address the regional host the posting belongs to. Lever serves its
European tenants from a separate host, and a posting fetched from the wrong one does
not exist.

#### Scenario: A Lever posting is queued and captured

- **WHEN** ingest writes a Lever posting that has no stored form
- **THEN** a capture is queued for it, and the worker fetches and stores its form

#### Scenario: A European posting is fetched from the European host

- **WHEN** the worker captures a posting on Lever's European host
- **THEN** the request goes to that host rather than the default one

### Requirement: A group of radio inputs is captured as one question

Where a platform renders one question as several inputs sharing a single submit name
— the shape a radio group takes — the capture SHALL hold one control carrying every
alternative as an option, not one control per alternative.

Each option SHALL take its value from what the input submits and its label from the
text presented beside it, which are different strings.

#### Scenario: A yes/no question is one control

- **WHEN** a question is rendered as two radio inputs sharing one name, offering
  "Yes" and "No"
- **THEN** the capture holds one control with two options

### Requirement: A required marker is not part of the question

Where a platform marks a required question by decorating its label, the capture SHALL
record the requirement as the control's flag and SHALL NOT leave the decoration in the
question text.

#### Scenario: The marker does not reach the text

- **WHEN** a required question's label carries a glyph marking it required
- **THEN** the captured question reads as the employer wrote it and the control is
  recorded as required
