# apply-form-capture Specification

## Purpose
TBD - created by archiving change collect-ats-apply-forms. Update Purpose after archive.
## Requirements
### Requirement: A job's application form is stored as the ATS published it

The system SHALL store, for a job whose platform exposes one, the application
form a candidate would have to complete, holding at most one current form per
job. A stored form SHALL carry, for every control the platform described: the
identifier the platform itself uses for that control when an application is
submitted, the control's type, whether an answer is required, the question text,
and — where the platform enumerates the permitted answers — the full option list
with the value the platform expects for each option.

Field identifiers, option values and question text SHALL be stored verbatim as
the platform returned them. The store SHALL NOT normalize them into a
freehire-internal vocabulary, because their only purpose is to be handed back to
that same platform.

#### Scenario: A form with enumerated answers is stored whole

- **WHEN** a platform describes a required single-choice question with three
  permitted answers
- **THEN** the stored form carries that question's submit identifier, its type,
  its required flag, its text, and all three options with the value the platform
  expects for each

#### Scenario: A second capture replaces the first

- **WHEN** a job's form is captured and that job's form is captured again later
- **THEN** the job carries exactly one stored form, the later one

#### Scenario: A job whose platform exposes no form has none

- **WHEN** a job's provider is one for which no form can be read
- **THEN** no form is stored for that job and no empty form row is created

### Requirement: An adapter that already receives the form yields it with the job

Where a platform's list endpoint already carries the application form, the
provider's adapter SHALL yield that form together with the job it belongs to, and
the ingest write path SHALL persist it. Such an adapter SHALL NOT issue any
additional request to obtain the form.

#### Scenario: Recruitee's form rides along with the posting

- **WHEN** the Recruitee adapter fetches a board and the platform returns an
  offer carrying its open questions and its standard-field requirements
- **THEN** the adapter yields that form with the job, and ingest stores it,
  having made no request beyond the board listing it already performs

#### Scenario: An offer with no questions still records its standard fields

- **WHEN** a Recruitee offer declares no open questions but does declare that a
  CV and a phone number are required and a cover letter is optional
- **THEN** a form is stored recording those three standard-field requirements

### Requirement: A form that needs its own request is captured through a queue

Where a platform exposes the form only through a per-posting request, the system
SHALL capture it asynchronously rather than during the crawl: the ingest write
path SHALL enqueue a capture for a job of such a provider, and a separate worker
SHALL perform the request. Enqueueing SHALL NOT delay or fail the ingest run.

A capture SHALL be enqueued only for a job that has no current form, so that a
posting is fetched once rather than once per ingest run.

#### Scenario: A newly ingested Greenhouse job is queued

- **WHEN** ingest writes a Greenhouse job that has no stored form
- **THEN** a capture for that job is queued and the ingest run completes without
  having contacted the form endpoint

#### Scenario: Re-ingesting an unchanged posting queues nothing

- **WHEN** ingest writes a Greenhouse job that already carries a stored form
- **THEN** no further capture is queued for it

#### Scenario: A provider with no readable form is never queued

- **WHEN** ingest writes a job whose provider exposes no readable form
- **THEN** no capture is queued for it

### Requirement: A worker drains the capture queue

A standalone run-once-and-exit worker SHALL claim queued captures, fetch each
job's form from its platform, store it, and exit. The worker SHALL bound how many
captures it performs concurrently, so that one run cannot flood a platform.

A capture that fails SHALL be recorded with its error and retried on a later run
up to a bounded number of attempts, after which it SHALL be marked failed and
left alone. One failing capture SHALL NOT abort the run or affect any other
capture.

#### Scenario: A queued capture is fetched and stored

- **WHEN** the worker runs with a queued capture for a Greenhouse job
- **THEN** it fetches that job's form, stores it, and removes the capture from
  the queue

#### Scenario: A failing capture is isolated and retried

- **WHEN** one queued capture's platform returns an error and others succeed
- **THEN** the successful captures are stored, the failing one records its error
  and remains eligible for a later run, and the worker's exit status reflects
  that the run itself completed

#### Scenario: A capture that keeps failing stops being retried

- **WHEN** a capture has failed the maximum permitted number of attempts
- **THEN** it is marked failed and no further run claims it

### Requirement: A stored form records where and when it came from

Every stored form SHALL record which provider it was read from and when it was
captured, so that a form's age is answerable and a provider's captures can be
re-run or discarded as a group when the platform changes its shape.

#### Scenario: A capture is attributable

- **WHEN** a form has been stored
- **THEN** it carries the provider it was read from and the time it was captured

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

