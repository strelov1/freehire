## Purpose

Lets the system submit a job application on a candidate's behalf, without the candidate
present, for a queued (candidate, job) attempt — completing it only when every question the
employer's form requires can be answered from what the candidate has already told the system.

## ADDED Requirements

### Requirement: An application is submitted only when every required question is answered

The system SHALL submit an application for a queued attempt only if every required question
on that job's live application form can be answered from the candidate's existing profile
data. If any required question cannot be answered, the system SHALL submit nothing for that
attempt and SHALL NOT partially fill the employer's form.

#### Scenario: Every required question is answered

- **WHEN** a queued attempt's job posting requires only questions the candidate's profile
  already answers (name, contact details, work authorization)
- **THEN** the application is submitted to the employer

#### Scenario: A required question has no known answer

- **WHEN** a queued attempt's job posting requires a question the candidate's profile does
  not cover
- **THEN** no application is submitted for that attempt, and the employer's form is left
  untouched

### Requirement: An unresolved attempt is recorded with the reason it could not proceed

The system SHALL record, for an attempt it could not submit, which required question(s) it
could not answer and why — so the gap can be closed and the attempt retried later.

#### Scenario: A parked attempt names its missing questions

- **WHEN** an attempt cannot be submitted because one or more required questions have no
  known answer
- **THEN** the attempt's record identifies each such question

### Requirement: The employer's rendered form is authoritative over its declared question list

Where an ATS both renders an application form and separately publishes a machine-readable list
of its questions, and the two disagree about what a candidate must answer, the system SHALL
follow what the rendered form actually asks.

#### Scenario: A rendered field is required but absent from the platform's own question list

- **WHEN** a job's application form renders a required question that the platform's published
  question list does not mention
- **THEN** the system treats that question as required and does not submit unless it is
  answered

### Requirement: A successful automated submission is recorded the same way a manual one is

A submission the system makes on a candidate's behalf SHALL be reflected in the candidate's
application record and history identically to an application the candidate submitted
themselves, so nothing about the candidate-facing record distinguishes how the application was
submitted.

#### Scenario: An automated submission appears in the candidate's application history

- **WHEN** the system submits an application for a queued attempt
- **THEN** the candidate's application record shows that job as applied, with the same
  observable history entries a self-submitted application would produce

### Requirement: An attempt the system could not resolve does not change the application's tracked stage

Failing to submit an application SHALL NOT move that job's tracked application stage for the
candidate. Only a completed submission does.

#### Scenario: A parked attempt leaves the candidate's tracked stage untouched

- **WHEN** an attempt cannot be submitted because a required question has no known answer
- **THEN** the job's tracked application stage for that candidate is unchanged from before the
  attempt

### Requirement: The same application is never submitted twice

The system SHALL NOT submit more than one application for the same (candidate, job) pair.

#### Scenario: A second attempt for an already-submitted application is not submitted again

- **WHEN** an attempt is processed for a (candidate, job) pair that already has a submitted
  application
- **THEN** no second submission is made to the employer

### Requirement: A transient failure is retried rather than treated as a permanent outcome

If an attempt cannot be completed because of a transient failure (a network or infrastructure
problem, not the absence of a required answer), the system SHALL retry it rather than
recording it as either submitted or permanently unresolved.

#### Scenario: A network failure during an attempt is retried

- **WHEN** an attempt fails because of a transient infrastructure problem before a submit or
  park outcome is reached
- **THEN** the attempt is retried rather than left in a final state
