## ADDED Requirements

### Requirement: Auto-apply attempts have an ordered history
Every state transition of an auto-apply attempt SHALL be recorded as an event keyed by
(user, job), and the history SHALL survive the attempt's deletion.

#### Scenario: A park records its questions
- **WHEN** an attempt is parked on unanswered fields
- **THEN** a `blocked` event is recorded carrying the field labels and no internal error text

#### Scenario: Submission is recorded
- **WHEN** the worker submits an attempt and retires its queue row
- **THEN** a `submitted` event remains for that (user, job)

### Requirement: A stuck attempt can be retried
A blocked or failed attempt SHALL be re-armable by its owner, except a declined attempt and
an attempt whose submission may already have reached the employer.

#### Scenario: Retry after answering
- **WHEN** an approved attempt parked on a question is retried
- **THEN** it becomes claimable by the submit pass again and a `retried` event is recorded

#### Scenario: Uncertain submission
- **WHEN** an attempt was dead-lettered because its submission could not be confirmed
- **THEN** retry is refused

### Requirement: An attempt can be cancelled
An owner SHALL be able to cancel an attempt that is not currently being submitted, except an
attempt whose submission may already have reached the employer — its row is the only record
that stops a fresh start from sending it twice.

#### Scenario: Cancel an uncertain submission
- **WHEN** the attempt was dead-lettered because its submission could not be confirmed
- **THEN** cancel is refused

#### Scenario: Cancel mid-submission
- **WHEN** the attempt is approved and claimed within the worker lease
- **THEN** cancel is refused

### Requirement: MCP drives auto-apply under an account switch
The signed-in MCP server SHALL offer tools to start, list, inspect, review, retry and cancel
attempts and to read and write screening answers; the tools that start, review, retry or
cancel SHALL refuse unless the account enabled `agents_may_apply` from a browser session.

#### Scenario: Switch off
- **WHEN** an MCP client calls `auto_apply_start` and the account has not enabled agents
- **THEN** the call fails with a message telling the person where to enable it

#### Scenario: Answer through MCP is sendable
- **WHEN** an MCP client saves an answer with `answer_bank_save`
- **THEN** it is stored with candidate provenance and is used by the next attempt
