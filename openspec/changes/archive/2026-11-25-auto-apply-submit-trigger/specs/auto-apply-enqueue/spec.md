## Purpose

Lets a candidate start an auto-apply attempt for one job posting from the job page itself,
creating the durable queue entry the tailor-then-review sequence already knows how to
carry to completion.

## ADDED Requirements

### Requirement: Only Greenhouse-sourced postings may be enqueued

The system SHALL refuse to enqueue an auto-apply attempt for a job whose source is not
Greenhouse.

#### Scenario: A non-Greenhouse job is refused

- **WHEN** a candidate requests auto-apply for a job whose source is not `greenhouse`
- **THEN** the system refuses and creates no queue entry

#### Scenario: A Greenhouse job is accepted

- **WHEN** a candidate requests auto-apply for a job whose source is `greenhouse`
- **THEN** the system proceeds to the remaining eligibility checks

### Requirement: Only a PRO-plan candidate may enqueue an attempt

The system SHALL refuse to enqueue an auto-apply attempt for a candidate who is not on the
PRO plan tier at the time of the request.

#### Scenario: A free-tier candidate is refused

- **WHEN** a candidate not on the PRO plan requests auto-apply for an eligible job
- **THEN** the system refuses and creates no queue entry

### Requirement: A base CV must exist before an attempt is enqueued

The system SHALL refuse to enqueue an auto-apply attempt for a candidate with no base CV.

#### Scenario: A candidate with no CV is refused

- **WHEN** a candidate with no base CV requests auto-apply for an eligible job
- **THEN** the system refuses and creates no queue entry

### Requirement: One durable attempt per candidate and job, created at most once

The system SHALL create at most one auto-apply queue entry per candidate and job. A repeat
request for a pair that already has a live entry SHALL succeed without creating a second
entry. A repeat request for a pair whose entry was already declined by the candidate SHALL
be refused permanently.

#### Scenario: A fresh request creates one entry

- **WHEN** an eligible candidate requests auto-apply for a job with no existing entry for
  that pair
- **THEN** the system creates exactly one queue entry for that candidate and job

#### Scenario: A repeat request before any decision is idempotent

- **WHEN** an eligible candidate requests auto-apply for a job for which they already have
  a live, undecided queue entry
- **THEN** the system reports success without creating a second entry

#### Scenario: A repeat request after a decline is refused permanently

- **WHEN** a candidate requests auto-apply for a job for which their own prior queue entry
  was declined
- **THEN** the system refuses the request and creates no new entry

### Requirement: Enqueuing an attempt starts the tailor-then-review sequence

Creating a fresh queue entry SHALL start the same durable, event-triggered sequence that
carries an auto-apply attempt through tailoring and the candidate's review.

#### Scenario: A fresh entry starts the sequence

- **WHEN** the system creates a fresh auto-apply queue entry for a candidate and job
- **THEN** the tailor-then-review sequence for that entry begins

### Requirement: A candidate can see their own auto-apply status for a job

The system SHALL let an authenticated candidate learn their own current auto-apply status
(no attempt, queued and undecided, or declined) for a specific job they are viewing.

#### Scenario: A candidate views a job they have not auto-applied to

- **WHEN** an authenticated candidate views a job with no auto-apply queue entry of their
  own
- **THEN** the system reports that no attempt exists for that candidate and job

#### Scenario: A candidate views a job they already queued

- **WHEN** an authenticated candidate views a job for which they already have a live,
  undecided auto-apply queue entry
- **THEN** the system reports that attempt as queued and undecided

#### Scenario: A candidate views a job they declined

- **WHEN** an authenticated candidate views a job for which their own auto-apply attempt
  was declined
- **THEN** the system reports that attempt as declined
