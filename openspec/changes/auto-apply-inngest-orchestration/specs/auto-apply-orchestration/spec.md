## Purpose

Durably sequences an auto-apply queue entry's tailor call and its candidate-review call,
pausing for however long the candidate's decision takes — including across process
restarts — without polling or a second implementation of either step.

## ADDED Requirements

### Requirement: Submitting an entry starts a durable tailor-then-review sequence

The system SHALL start one durable, event-triggered run per auto-apply queue entry
submission, carrying the entry's own identifier through every step of that run. The run
SHALL call the existing tailoring trigger for that entry as its first step.

#### Scenario: A submission starts the sequence

- **WHEN** a submission event is published for one auto-apply queue entry
- **THEN** a durable run starts for that entry and calls the tailoring trigger for it

### Requirement: The run pauses for however long the candidate's decision takes

After tailoring completes, the run SHALL pause — without polling, without holding a
process, without a wall-clock upper bound shorter than several days — until a decision
event arrives for the SAME entry, and then resume and complete with that decision. The
decision event is published only after the review-decision endpoint has already durably
recorded the decision, so the run SHALL NOT call that endpoint again on resume — doing so
would always be refused as an already-reviewed entry. A decision event for a DIFFERENT
entry SHALL NOT resume this run.

#### Scenario: A decision arrives minutes after tailoring finishes

- **WHEN** tailoring for an entry finishes and a decision event for that same entry is
  published shortly afterward
- **THEN** the run resumes and completes with that decision, without calling the
  review-decision endpoint again

#### Scenario: A decision arrives long after tailoring finishes, across a process restart

- **WHEN** tailoring for an entry finishes, the orchestrating process restarts, and a
  decision event for that entry is published after the restart
- **THEN** the run still resumes from where it paused and completes with that decision —
  nothing about the pause is lost by the restart

#### Scenario: A decision for a different entry does not resume this run

- **WHEN** a run is paused awaiting entry A's decision and a decision event for entry B is
  published
- **THEN** entry A's run remains paused

### Requirement: A failed or timed-out step ends the run without a partial write

A tailoring call that fails SHALL end the run without ever recording a review decision. A
pause that exceeds its own bound without a matching decision event SHALL end the run the
same way — without recording a review decision and without retrying the tailoring call.
Either outcome SHALL leave the entry exactly as the tailoring/review endpoints themselves
already leave an unresolved entry, for a human to notice later.

#### Scenario: A tailoring failure never reaches the review step

- **WHEN** the tailoring call for an entry fails
- **THEN** the run ends without calling the review step for that entry

#### Scenario: A pause that exceeds its bound ends without a decision

- **WHEN** a run has been paused awaiting a decision for longer than its own bound and no
  matching decision event has arrived
- **THEN** the run ends without recording a review decision and without starting a new
  tailoring call

### Requirement: Publishing a decision event is best-effort and never blocks the caller

Recording a candidate's review decision (the existing review endpoint's own write) SHALL
NOT depend on, wait for, or fail because of publishing the decision event that resumes a
paused run. A publish failure SHALL be observable to operators without being surfaced to
the caller as a request failure.

#### Scenario: The decision is recorded even if the event publish fails

- **WHEN** a candidate's review decision is recorded and publishing the corresponding
  decision event fails
- **THEN** the decision is still recorded and the request that recorded it does not fail
  because of the publish failure
