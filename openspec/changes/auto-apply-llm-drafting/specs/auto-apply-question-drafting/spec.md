## Purpose

Answers a custom, free-text application question the deterministic resolver could not,
by drafting a short answer grounded strictly in the candidate's own stored data — never
inventing a fact, and never drafting a sensitive question regardless of confidence.

## ADDED Requirements

### Requirement: A sensitive question is never drafted

The system SHALL NOT generate a drafted answer for a question whose text matches the
sensitive-field vocabulary (compensation, work authorization/visa sponsorship, or an
equal-opportunity/demographic category), regardless of how well-grounded a draft would be.
The check SHALL run before any model call is made for that question.

#### Scenario: A visa-sponsorship question is never drafted

- **WHEN** an unmapped required question's text concerns visa sponsorship or work
  authorization
- **THEN** the system does not attempt to draft an answer for it, and no model call is made
  for that question

#### Scenario: An equal-opportunity question is never drafted

- **WHEN** an unmapped question's text concerns a demographic or equal-opportunity category
  (gender, race/ethnicity, veteran status, disability, sexual orientation)
- **THEN** the system does not attempt to draft an answer for it

### Requirement: A drafted answer is grounded only in the candidate's own asserted data

A drafted answer SHALL be produced only from facts the candidate themselves is the source
of — never invented, and never drawn from a fact whose provenance indicates the system
inferred it rather than the candidate stating it.

#### Scenario: A draft may use a candidate-stated fact

- **WHEN** the candidate's stored experience record contains a fact they themselves entered,
  imported from their CV, or stated directly
- **THEN** a draft may reference that fact

#### Scenario: A draft may not use a system-inferred fact

- **WHEN** the candidate's stored experience record contains a fact the system inferred on
  the candidate's behalf, not one the candidate stated
- **THEN** a draft SHALL NOT reference that fact, the same way such a fact may not appear on
  a candidate's CV

### Requirement: A non-sensitive question with no groundable answer still parks

Drafting SHALL NOT invent an answer where the candidate's stored data gives no basis for
one. A required question that cannot be answered — by the deterministic resolver or by a
grounded draft — SHALL still be reported unmapped, exactly as an unresolved question is
today.

#### Scenario: No groundable fact exists for a required free-text question

- **WHEN** a required, non-sensitive free-text question has no known-answer match and the
  candidate's stored data gives no basis for a grounded draft
- **THEN** the question is reported unmapped and the attempt is not submitted

### Requirement: A drafted answer for an enumerated question still must match an offered option

Where a question offers a fixed set of choices, a drafted answer SHALL still be checked
against the platform's own offered options before it is used — drafting does not bypass the
"never answer with an option the platform did not offer" rule the deterministic resolver
already enforces.

#### Scenario: A drafted choice matches no offered option

- **WHEN** a question offers enumerated choices and the best available drafted answer
  matches none of them
- **THEN** the question is reported unmapped rather than answered with an unoffered choice
