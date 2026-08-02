## ADDED Requirements

### Requirement: Workable's standard profile is told apart from the employer's questions

The display projection SHALL treat a Workable control as the employer's own question
only when the platform marks it as one, and SHALL treat every other control as part of
the standard profile every Workable application collects.

Workable marks an employer's question by prefixing its identifier, which is the
platform's own convention rather than an inference from the question's wording.

#### Scenario: An employer's question is shown

- **WHEN** a captured Workable form carries a control the platform marked as an
  employer question
- **THEN** the projection shows it among the questions

#### Scenario: The standard profile is collapsed

- **WHEN** a captured Workable form carries the name, email, phone, CV, education and
  experience controls
- **THEN** none of them appear as questions, and all appear once in the standard entry
