## Purpose

Gives the candidate a way, inside the tracker they already use, to see where every one of their
auto-apply attempts stands, act on one that needs a decision, and see exactly what would be
submitted on their behalf — without exposing any internal diagnostic detail that was never meant
for them to read, and without implying a fix exists where none does.

## ADDED Requirements

### Requirement: Starting auto-apply puts the job on the tracker board
The system SHALL ensure the job is tracked, at stage `preparing`, when an auto-apply attempt is
started for it, if it is not already tracked. This SHALL NOT change the stage of a job that is
already tracked.

#### Scenario: Auto-apply started for an untracked job
- **WHEN** the candidate triggers auto-apply for a job that does not yet appear on their tracker
  board
- **THEN** the job appears on the board at stage `preparing`

#### Scenario: Auto-apply started for an already-tracked job
- **WHEN** the candidate triggers auto-apply for a job already tracked at some other stage
- **THEN** the job's existing stage is left unchanged

### Requirement: A tracked job reports its auto-apply status alongside its other tracker data
The system SHALL derive, for a job with a live auto-apply attempt, exactly one status from:
`tailoring` (no tailored CV produced yet), `pending_review` (a tailored CV and answer preview
exist and await the candidate's decision), `approved` (the candidate approved it and it is queued
for submission), `blocked` (an unattended submission attempt could not answer a required
question), `declined` (the candidate declined the tailored CV), or `failed` (an unattended
submission attempt exhausted its retries). A job with no live auto-apply attempt SHALL report no
auto-apply status at all.

#### Scenario: An attempt awaiting the candidate's decision
- **WHEN** an attempt has a tailored CV and a resolved answer preview but no recorded review
  decision
- **THEN** the tracked job reports auto-apply status `pending_review`

#### Scenario: An attempt stuck on a required question
- **WHEN** an unattended submission attempt could not answer a required application question
- **THEN** the tracked job reports auto-apply status `blocked`, regardless of whether the
  candidate had approved it

#### Scenario: A declined attempt is never reported as blocked
- **WHEN** the candidate declined an attempt's tailored CV
- **THEN** the tracked job reports auto-apply status `declined`, even though declining also marks
  the attempt as blocked internally

#### Scenario: A job with no auto-apply attempt
- **WHEN** a tracked job has never had an auto-apply attempt started for it
- **THEN** its response carries no auto-apply status

### Requirement: A pending-review attempt carries an exact preview of what would be submitted
When an attempt's status is `pending_review`, the system SHALL include the previously-resolved
application-form answer preview and a reference to the tailored CV. This preview SHALL be the
same one the real, unattended submission would use — not recomputed or approximated at read time.

#### Scenario: Reviewing a pending attempt shows its resolved answers
- **WHEN** the candidate views a `pending_review` attempt
- **THEN** the response includes the answer preview computed when the attempt was tailored, and a
  reference to the tailored CV

### Requirement: A blocked attempt exposes what stopped it, never why the system failed
When an attempt's status is `blocked`, the system SHALL include the specific application
questions that could not be answered (candidate-facing, structured detail). The system SHALL NOT
include the attempt's internal failure/error text in any attempt's response, regardless of status.

#### Scenario: Blocked attempt names its unanswered questions
- **WHEN** an attempt is blocked because a required application question had no known answer
- **THEN** the response for that attempt includes the list of unanswered questions

#### Scenario: Internal error text is never surfaced
- **WHEN** an attempt has internal diagnostic error text recorded against it (blocked or failed)
- **THEN** that internal error text does not appear anywhere in the response

### Requirement: The candidate can approve or decline a pending-review attempt from the tracker
The system SHALL let the candidate record an approve/decline decision for a `pending_review`
attempt from the tracker, using the existing review decision behavior unchanged.

#### Scenario: Approving from the tracker
- **WHEN** the candidate approves a `pending_review` attempt from the tracker
- **THEN** the decision is recorded the same way the existing review endpoint already records an
  approval, and the attempt's tracker status becomes `approved`

#### Scenario: Declining from the tracker
- **WHEN** the candidate declines a `pending_review` attempt from the tracker
- **THEN** the decision is recorded the same way the existing review endpoint already records a
  decline, and the attempt's tracker status becomes `declined`

### Requirement: A terminal auto-apply status exposes no retry action
When an attempt's status is `blocked`, `declined`, or `failed`, the system SHALL NOT expose any
action to retry or re-enqueue that attempt, since no such path exists for a given job.

#### Scenario: A blocked attempt exposes no retry action
- **WHEN** the candidate views a `blocked` attempt
- **THEN** the response and its rendering offer no way to retry or re-enqueue that attempt
