# application-from-mail Specification

## Purpose
TBD - created by archiving change inbox-link-triage. Update Purpose after archive.
## Requirements
### Requirement: Recording an application from a piece of mail

The system SHALL let an authenticated caller record a tracked application from
one of their emails and link that email to it in a single action, given the
email and the public slug of a job in the catalog. This is the path for mail
about an application the caller never recorded: the employer is known and the
job is in the catalog, but no tracked application exists to attach the mail to.
Authentication MAY be by session cookie or by full-scope API key.

The action SHALL reuse the ordinary apply path, so the application is
indistinguishable from one recorded at the time of applying — it carries the same
stage seeding and participates in the same counting.

#### Scenario: Creating an application from unlinked mail

- **WHEN** an authenticated caller invokes the action on an unlinked email,
  naming a catalog job by its public slug
- **THEN** a tracked application is created for that caller and job
- **AND** the email becomes linked to it
- **AND** the response carries the resulting interaction record

#### Scenario: The link is recorded as manual

- **WHEN** an application is created from an email this way
- **THEN** the email's link is marked manual, not automatic — a caller's decision
  is never recorded as the matcher's

#### Scenario: Unknown slug changes nothing

- **WHEN** the caller names a slug that resolves to no job
- **THEN** the request is rejected with a 404
- **AND** no application is created and the email stays unlinked

#### Scenario: Mail belonging to another user

- **WHEN** the caller names an email that is not theirs
- **THEN** the request is rejected with a 404 and nothing is created

#### Scenario: Requires authentication

- **WHEN** the request carries neither a valid session cookie nor a valid
  full-scope API key
- **THEN** the system responds 401 and records nothing

### Requirement: The application is dated by the mail, not by the recording

The system SHALL set the created application's `applied_at` to the originating
email's `received_at`, not to the current time. The application demonstrably
existed by the moment the employer wrote, so the mail's timestamp is the honest
upper bound on when it was made. Dating it "now" would compress the application's
real history and make any elapsed-time reading of it wrong.

#### Scenario: Applied date comes from the email

- **WHEN** an application is created from an email received three weeks ago
- **THEN** the application's `applied_at` is that email's `received_at`, not the
  current time

#### Scenario: The stage is seeded as for any application

- **WHEN** an application is created from an email
- **THEN** its stage is seeded exactly as marking a job applied seeds it

### Requirement: An existing application is linked, not duplicated

The system SHALL link the email to the caller's existing interaction when one
already exists for that `(caller, job)` pair, rather than creating a second one
or failing. The `(user, job)` pair admits at most one interaction, so this action
is an upsert like every other per-user job write. An interaction that already
carries an `applied_at` SHALL keep it — a later recording never rewrites an
earlier application's date.

#### Scenario: Job already tracked but not applied to

- **WHEN** the caller invokes the action for a job they had only viewed or saved
- **THEN** the existing interaction gains `applied_at` from the email
- **AND** no second interaction row is created

#### Scenario: Job already applied to keeps its original date

- **WHEN** the caller invokes the action for a job they already marked applied
- **THEN** the existing `applied_at` is left unchanged
- **AND** the email is still linked to that application

#### Scenario: Repeating the action is idempotent

- **WHEN** the caller invokes the action twice for the same email and job
- **THEN** the second call changes nothing and does not error

### Requirement: A late recording counts as an application

The system SHALL count an application created from mail exactly as it counts one
recorded at apply time, incrementing the job's materialized `applied_count` when
— and only when — `applied_at` transitions from unset to set. A late recording is
the same act as an ordinary apply, so it neither escapes the count nor inflates
it on repetition.

#### Scenario: First recording increments the count

- **WHEN** an application is created from mail for a job the caller had not
  applied to
- **THEN** that job's `applied_count` is incremented by one

#### Scenario: Recording an already-applied job does not double-count

- **WHEN** the caller invokes the action for a job whose interaction already has
  `applied_at` set
- **THEN** the job's `applied_count` is not incremented again

### Requirement: Mail with a pending suggestion is confirmed, not overridden

The system SHALL reject this action for an email that carries a pending
suggestion, directing the caller to confirm or reject that suggestion instead.
The matcher has already proposed an answer for such mail; letting a second path
silently overwrite it would make the resulting link's provenance ambiguous.

#### Scenario: Email with a pending suggestion

- **WHEN** the caller invokes the action on an email carrying a pending
  suggestion
- **THEN** the request is rejected with a client error naming the pending
  suggestion
- **AND** no application is created and the suggestion is untouched

#### Scenario: Available again after the suggestion is rejected

- **WHEN** the caller rejects the pending suggestion and invokes the action again
- **THEN** the application is created and the email is linked to it

