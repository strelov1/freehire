## ADDED Requirements

### Requirement: Lever's standard application is told apart from the employer's questions

The display projection SHALL treat a Lever control as the employer's own question only
when the platform's submit name marks it as one, and SHALL treat the rest — the
candidate's name, contact details, CV, employer and profile links — as the standard
application every Lever posting collects.

#### Scenario: An employer's question is shown

- **WHEN** a captured Lever form carries a control whose submit name marks it as an
  employer question
- **THEN** the projection shows it among the questions

#### Scenario: The standard application is collapsed

- **WHEN** a captured Lever form carries the name, email, phone, CV and profile-link
  controls
- **THEN** none of them appear as questions, and each appears once in the standard entry
