## ADDED Requirements

### Requirement: An employer sees the applications sent to their vacancies
An active employer account SHALL be able to list the applications sent on freehire to the
vacancies it published, newest first, narrowed by vacancy and by stage, with the count of
applications at each stage under the vacancy narrowing. Only delivered applications SHALL be
listed, and none to another account's vacancies.

#### Scenario: Listing the inbox
- **WHEN** an active employer lists applications for one of their vacancies
- **THEN** each delivered application shows the candidate's email, answers, note, stage and when it was sent, and the meta counts every stage

#### Scenario: Another employer's applications stay hidden
- **WHEN** an employer lists, opens or moves an application to a vacancy another account owns
- **THEN** the list does not contain it and opening or moving it is answered as not found

#### Scenario: An undelivered application is not shown
- **WHEN** an application was recorded but its delivery has not succeeded
- **THEN** it is neither listed nor counted nor movable

#### Scenario: An unknown stage filter is refused
- **WHEN** the list is requested with a stage outside the pipeline
- **THEN** the request is refused as invalid rather than answered unfiltered

### Requirement: The employer opens the CV they were sent
The CV file sent with an application SHALL be kept as sent and served to the vacancy's
employer, inline and uncached. A later edit or deletion of the candidate's CV SHALL NOT change
it. An application sent before CVs were kept SHALL report that it has none.

#### Scenario: Opening a CV
- **WHEN** the employer opens the CV of a delivered application
- **THEN** they receive the exact file that was emailed, with its content type

#### Scenario: A retried application keeps the retry's CV
- **WHEN** a delivery fails and the candidate's retry succeeds with a different CV
- **THEN** the employer's inbox serves the CV of the attempt that was delivered

#### Scenario: The copy goes with the candidate's account
- **WHEN** the candidate deletes their account
- **THEN** the kept CV is deleted with the application

### Requirement: The employer moves an application through a pipeline
An active employer SHALL be able to move a delivered application to one of the stages new,
reviewing, interview, offer, hired, rejected. A move to the stage it already holds SHALL
change nothing and notify nobody.

#### Scenario: Moving an application
- **WHEN** the employer moves an application to interview
- **THEN** its stage is interview and its stage change time is set

#### Scenario: A repeated move
- **WHEN** the employer moves an application to the stage it already holds
- **THEN** nothing changes and no email is sent

### Requirement: The candidate sees the employer's decision
Every move SHALL be mirrored onto the candidate's own tracking (new as applied, reviewing as
screening, hired as accepted, the others by name) and recorded in the application-event
ledger with source `employer`. The mirror SHALL NOT move a stage the candidate set backward,
SHALL NOT reopen an application the candidate settled, and SHALL NOT recreate an application
the candidate removed or whose progress they cleared; a stage the employer set SHALL be correctable by the employer in any
direction.

#### Scenario: An interview reaches the candidate's tracking
- **WHEN** the employer moves an applied candidate to interview
- **THEN** the candidate's tracking shows interview, recorded with source employer

#### Scenario: A withdrawal is never reopened
- **WHEN** the candidate has withdrawn and the employer moves the application to offer
- **THEN** the candidate's tracking still shows withdrawn

#### Scenario: The employer corrects their own rejection
- **WHEN** the employer rejected an application and then moves it back to interview
- **THEN** the candidate's tracking shows interview

### Requirement: Interview, offer and rejection email the candidate
A move to interview, offer or rejected SHALL email the candidate, Reply-To the employer
account's work email, quoting the employer's optional message of at most 2000 characters.
A message SHALL be refused with any other stage. The move SHALL be saved even when the email
fails, and the response SHALL say whether the candidate was emailed.

#### Scenario: A rejection with a message
- **WHEN** the employer rejects an application with a message
- **THEN** the candidate receives an email quoting it, and replying reaches the employer

#### Scenario: A message with a stage that sends no letter
- **WHEN** the employer moves an application to reviewing with a message
- **THEN** the request is refused as invalid and nothing changes

#### Scenario: The email fails
- **WHEN** the letter to the candidate cannot be sent
- **THEN** the stage is still moved and the response reports the candidate was not emailed
