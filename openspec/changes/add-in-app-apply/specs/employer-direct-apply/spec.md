## ADDED Requirements

### Requirement: An employer vacancy may accept applications on freehire
An active employer account SHALL be able to publish a vacancy in one of two apply modes:
`external` (an apply URL is required, today's behaviour) or `freehire` (no apply URL is
accepted). The apply mode SHALL NOT change after the vacancy is created.

#### Scenario: Publishing an on-site vacancy
- **WHEN** an active employer creates a vacancy with apply mode `freehire` and no URL
- **THEN** the vacancy is published under the account's company and has no external apply URL

#### Scenario: An on-site vacancy refuses a URL
- **WHEN** an active employer creates a vacancy with apply mode `freehire` and a URL
- **THEN** the request is refused as invalid

#### Scenario: An external vacancy still requires a URL
- **WHEN** an active employer creates a vacancy with apply mode `external` and no URL
- **THEN** the request is refused as invalid

#### Scenario: Apply mode is fixed
- **WHEN** an employer edits an existing vacancy and supplies a different apply mode
- **THEN** the request is refused as invalid and the vacancy keeps its mode

### Requirement: An on-site vacancy carries the employer's questions
A `freehire`-mode vacancy SHALL carry zero to ten employer questions. Each question SHALL
have a non-empty label, a type of `text`, `boolean`, or `select`, and a required flag. A
`select` question SHALL carry two to twenty distinct non-empty options. The questions SHALL
be stored as the vacancy's `apply_forms` row with provider `freehire`, and only the vacancy's
own employer SHALL be able to change them.

#### Scenario: Questions are saved with the vacancy
- **WHEN** an employer creates a `freehire` vacancy with a required boolean question and an optional text question
- **THEN** reading that vacancy's apply form returns both questions with their types and required flags

#### Scenario: A malformed select question is refused
- **WHEN** an employer submits a `select` question with fewer than two options
- **THEN** the request is refused as invalid and nothing is saved

#### Scenario: Another employer cannot change the questions
- **WHEN** an employer edits the questions of a vacancy a different employer account owns
- **THEN** the request is answered as not found and the questions are unchanged

#### Scenario: An external vacancy takes no questions
- **WHEN** an employer submits questions for an `external`-mode vacancy
- **THEN** the request is refused as invalid

### Requirement: A candidate applies to an on-site vacancy
A signed-in user with a verified email SHALL be able to apply to an open `freehire`-mode
vacancy whose employer account is active, by sending exactly one CV — the CV they tailored
for THAT vacancy, or the CV file they uploaded to their profile — answering the questions,
and optionally adding a note of at most 2000 characters. When they have a CV tailored for the
vacancy, the form SHALL attach it by default; with neither, it SHALL offer to upload a CV or
to tailor one. No other CV (a base CV, or one tailored for another vacancy) SHALL be offered
or accepted. Every required question
SHALL be answered; a `text` answer SHALL be at most 2000 characters; a `boolean` answer SHALL be true or false; a `select` answer SHALL be one
of that question's options; an answer to a question the form does not hold SHALL be refused.

#### Scenario: A complete application is accepted
- **WHEN** a verified user applies with their own CV and an answer to every required question
- **THEN** the application is recorded and delivered to the employer

#### Scenario: A missing required answer is refused
- **WHEN** a verified user applies without answering a required question
- **THEN** the request is refused as invalid, naming the question, and nothing is recorded

#### Scenario: An answer outside a select's options is refused
- **WHEN** a verified user answers a `select` question with a value not among its options
- **THEN** the request is refused as invalid and nothing is recorded

#### Scenario: An unverified user cannot apply
- **WHEN** a signed-in user whose email is not verified applies
- **THEN** the request is refused as forbidden

#### Scenario: Someone else's CV cannot be sent
- **WHEN** a user applies naming a CV that belongs to another user, or one of their own tailored for another vacancy
- **THEN** the request is answered as not found and nothing is recorded

#### Scenario: The uploaded CV is sent as uploaded
- **WHEN** a user applies with the CV file they uploaded to their profile
- **THEN** the employer receives that file, and the record says the uploaded CV was sent

#### Scenario: The tailored CV is attached by itself
- **WHEN** a user who tailored a CV for the vacancy opens the application form
- **THEN** that CV is attached, and they may switch to their uploaded CV instead

#### Scenario: A closed vacancy takes no applications
- **WHEN** a user applies to a `freehire` vacancy that is closed
- **THEN** the request is refused as gone and nothing is recorded

#### Scenario: A revoked employer takes no applications
- **WHEN** a user applies to a vacancy whose employer account is no longer active
- **THEN** the request is refused as gone and nothing is recorded

#### Scenario: An employer does not apply to their own vacancy
- **WHEN** the employer who published a `freehire` vacancy applies to it
- **THEN** the request is answered as not found and nothing is recorded

#### Scenario: An external vacancy has no on-site application
- **WHEN** a user applies on freehire to an `external`-mode or non-employer vacancy
- **THEN** the request is answered as not found

### Requirement: An application is delivered once to the employer's confirmed address
The application SHALL be emailed to the employer account's confirmed work email, with the
chosen CV attached as a PDF, the questions and answers and the note in the body, and
`Reply-To` set to the candidate's email. The application SHALL be recorded before delivery
and stamped sent only after delivery succeeds. A user SHALL have at most one application per
vacancy: applying again to a vacancy already sent is refused as a conflict, and applying
again to one recorded but not sent replaces its content with the new submission and retries
delivery.

#### Scenario: Reply goes to the candidate
- **WHEN** an application is delivered
- **THEN** the email's recipient is the employer's confirmed work email and its Reply-To is the candidate's email

#### Scenario: A failed delivery is retried, not duplicated
- **WHEN** delivery of an application fails and the candidate applies again to the same vacancy
- **THEN** the recorded application is delivered and exactly one application exists for that candidate and vacancy

#### Scenario: A failed delivery is not reported as sent
- **WHEN** delivery of an application fails
- **THEN** the request fails, the application stays unsent, and the candidate's tracking is not marked applied

#### Scenario: A second application is refused
- **WHEN** a candidate applies to a vacancy their application was already sent to
- **THEN** the request is refused as a conflict and no email is sent

### Requirement: A sent application updates the candidate's records
After delivery succeeds, the vacancy SHALL be marked applied in the candidate's tracking and
each answer whose question resolves to an answer-bank topic SHALL be saved to the candidate's
answer bank as candidate-provided. A failure to save an answer SHALL NOT fail the application.

#### Scenario: Tracking shows the application
- **WHEN** an application is delivered
- **THEN** the vacancy appears as applied in the candidate's tracking

#### Scenario: Answers prefill the next form
- **WHEN** a candidate has answered a question on one on-site application and opens another vacancy asking the same topic
- **THEN** the form offers the earlier answer as the prefilled value

### Requirement: Applications are capped per candidate per day
A candidate SHALL be able to submit at most `DIRECT_APPLY_DAILY_LIMIT` applications per UTC
day (default 30). An application counts from the moment it is recorded, whether or not its
delivery has succeeded yet — counting only delivered ones would let a burst of in-flight or
failing submissions pass the cap. A retry of an unsent application on the same vacancy SHALL
NOT count as a new one. An unparseable
or non-positive value SHALL fail server startup rather than fall back.

#### Scenario: The cap refuses the next application
- **WHEN** a candidate who has sent the daily limit of applications today applies to another vacancy
- **THEN** the request is refused as too many requests and nothing is recorded

### Requirement: The job page presents an on-site vacancy
A `freehire`-mode vacancy SHALL be shown with an "Only on freehire" badge and an Apply action
that opens the on-site form instead of an external link. Before sending, the form SHALL state
which company will receive the CV and answers. Its `JobPosting` structured data SHALL declare
`directApply: true`; no other posting SHALL declare it.

#### Scenario: Structured data declares direct apply
- **WHEN** the page of a `freehire`-mode vacancy is rendered
- **THEN** its `JobPosting` JSON-LD contains `"directApply": true`

#### Scenario: An external posting does not declare direct apply
- **WHEN** the page of any other posting is rendered
- **THEN** its `JobPosting` JSON-LD does not contain `directApply: true`

#### Scenario: An applied candidate sees it
- **WHEN** a candidate whose application to the vacancy was delivered opens it
- **THEN** the page shows that the application was sent instead of the Apply action

#### Scenario: A hand-marked application does not block applying
- **WHEN** a candidate marked the vacancy applied in their tracking but never applied on freehire
- **THEN** the page still offers the Apply action

### Requirement: An employer's vacancy is findable and reversible
An employer vacancy SHALL require a description, on create and on an edit, because search
keeps out a posting without one. Search SHALL NOT exclude an employer vacancy for a title no
dictionary resolves. An employer SHALL be able to reopen a vacancy they closed; a vacancy a
moderator or another mechanism closed SHALL stay closed. Closing SHALL ask for confirmation,
and the dashboard SHALL link each vacancy to its page.

#### Scenario: A vacancy without a description is refused
- **WHEN** an employer publishes a vacancy whose description is empty or only markup
- **THEN** the request is refused as invalid and nothing is published

#### Scenario: An unrecognised title stays searchable
- **WHEN** an open employer vacancy's title resolves to no category
- **THEN** it is still indexed for search and shown on its company page

#### Scenario: The employer reopens their own close
- **WHEN** an employer reopens a vacancy they closed
- **THEN** it is open again and queued for search, and a pending removal from search is dropped

#### Scenario: Another close is not the employer's to undo
- **WHEN** an employer reopens a vacancy a moderator closed, or one that is open
- **THEN** the request is refused as invalid and the vacancy is unchanged
