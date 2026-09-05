## Purpose

Gives the candidate a single, readable view of where every one of their auto-apply attempts
stands — including one they must act on and ones auto-apply has already completed — without
exposing any internal diagnostic detail that was never meant for them to read.

## ADDED Requirements

### Requirement: The list is scoped to the caller
The system SHALL return only the authenticated caller's own auto-apply attempts and recently
completed submissions. An unauthenticated request SHALL be rejected.

#### Scenario: Candidate has no auto-apply activity
- **WHEN** a signed-in candidate who has never started an auto-apply attempt requests their
  auto-apply list
- **THEN** the system returns an empty pending list and an empty recently-completed list,
  not an error

#### Scenario: Unauthenticated request
- **WHEN** a request carries no valid session
- **THEN** the system rejects it and returns no other candidate's data

### Requirement: Each live attempt reports exactly one of six statuses
The system SHALL derive, for every live auto-apply attempt, exactly one status from:
`tailoring` (no tailored CV produced yet), `pending_review` (a tailored CV exists and awaits
the candidate's decision), `approved` (the candidate approved it and it is queued for
submission), `blocked` (an unattended submission attempt could not answer a required
question), `declined` (the candidate declined the tailored CV), or `failed` (an unattended
submission attempt exhausted its retries). Exactly one status SHALL apply to a given attempt
at any time.

#### Scenario: An attempt awaiting the candidate's decision
- **WHEN** an attempt has a tailored CV but no recorded review decision
- **THEN** the system reports its status as `pending_review`

#### Scenario: An attempt stuck on a required question
- **WHEN** an unattended submission attempt could not answer a required application question
- **THEN** the system reports its status as `blocked`, regardless of whether the candidate
  had approved it

#### Scenario: A declined attempt is never reported as blocked
- **WHEN** the candidate declined an attempt's tailored CV
- **THEN** the system reports its status as `declined`, even though declining also marks the
  attempt as blocked internally

### Requirement: A blocked attempt exposes what stopped it, never why the system failed
When an attempt's status is `blocked`, the system SHALL include the specific application
questions that could not be answered (candidate-facing, structured detail). The system SHALL
NOT include the attempt's internal failure/error text in any attempt's response, regardless of
status.

#### Scenario: Blocked attempt names its unanswered questions
- **WHEN** an attempt is blocked because a required application question had no known answer
- **THEN** the response for that attempt includes the list of unanswered questions

#### Scenario: Internal error text is never surfaced
- **WHEN** an attempt has internal diagnostic error text recorded against it (blocked or
  failed)
- **THEN** that internal error text does not appear anywhere in the response

### Requirement: Recently completed submissions are listed separately from live attempts
The system SHALL list the caller's own recently completed auto-apply submissions (jobs an
unattended auto-apply attempt actually submitted on the candidate's behalf), most recent
first, bounded to a fixed count. This list SHALL include only submissions auto-apply itself
completed — not applications the candidate recorded manually or that arrived through mail
linking.

#### Scenario: A manual application is not shown as a completed auto-apply
- **WHEN** the candidate marks a job as applied themselves (not through auto-apply)
- **THEN** that application does not appear in the recently-completed auto-apply list

#### Scenario: A completed attempt no longer appears among live attempts
- **WHEN** an auto-apply attempt has been successfully submitted
- **THEN** it appears only in the recently-completed list, not among the live attempts
