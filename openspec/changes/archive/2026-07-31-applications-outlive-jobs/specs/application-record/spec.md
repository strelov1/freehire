## ADDED Requirements

### Requirement: An application is a record of the person's act, not of a catalogue row

The system SHALL store a tracked application as its own record carrying the applicant, the
employer as a company slug, the role title, `applied_at`, `stage`, `notes` and `followed_up_at`,
independent of whether the posting it was made against is still in the catalogue.

`applied_at` MAY be absent. The record's subject is the candidate's tracked process with an
employer, and applying is one event in it rather than its precondition: the tracker has always
allowed a stage to be set on a job never marked applied. A consumer that means "an application
was made" MUST test `applied_at`, never the row's existence.

The employer and the role title MUST be stored on the record itself rather than read through the
posting, because the posting is the part that can disappear.

The record MUST NOT carry its own provenance field. How an application came to exist is already
recorded by its `applied` event's source in the ledger, and a second copy would be a second thing
to keep true.

#### Scenario: A stage set without applying is still tracked

- **WHEN** a candidate sets a stage on a job they never marked applied
- **THEN** a record exists carrying that stage
- **AND** its `applied_at` is absent, so nothing reports it as an application

#### Scenario: Application carries the employer without consulting the catalogue

- **WHEN** an application is read
- **THEN** its company slug and role title come from the application record itself
- **AND** the read succeeds whether or not a posting is linked

#### Scenario: Provenance is read from the ledger

- **WHEN** a reader needs to know how an application came to exist
- **THEN** it reads the source of that application's `applied` event

### Requirement: The catalogue link is optional and at most one application per posting

An application MAY name the catalogue posting it was made against, and that link SHALL be
optional. While the link is present the system MUST allow at most one application per
`(user, posting)`, preserving today's guarantee that applying twice to the same posting updates
one record rather than creating a second. Applications with no posting linked MUST NOT be
constrained by that uniqueness, because two different roles at one employer are two applications.

#### Scenario: Applying twice to the same posting updates one record

- **WHEN** an authenticated user marks the same posting applied twice
- **THEN** exactly one application exists for that user and posting

#### Scenario: Two unlinked applications to the same employer coexist

- **WHEN** a user holds two applications to the same employer, neither linked to a posting
- **THEN** both records exist independently

### Requirement: An application outlives the posting it was made against

The system SHALL preserve an application when its linked posting is removed from the catalogue,
clearing only the link. `applied_at`, `stage`, `notes`, `followed_up_at`, the employer and the
role title MUST survive intact, and mail already linked to the application MUST remain linked.

This is the whole point of the record: a posting is inventory and is deleted on a schedule, while
an application is something a person did and cannot be deleted on our schedule.

#### Scenario: Pruning a posting leaves the application standing

- **WHEN** a posting a user applied to is deleted from the catalogue
- **THEN** the user's application still exists with its stage, notes and dates unchanged
- **AND** its catalogue link is cleared
- **AND** its linked mail is still attached to it

#### Scenario: Saved and viewed jobs still go with the posting

- **WHEN** a posting is deleted from the catalogue
- **THEN** the view, save, dismissal and vote a user recorded against it are removed as before

### Requirement: Aggregates correlate events through the application, not the posting

Every aggregate over the application-event ledger SHALL pair an application's events by the
application they belong to. Correlating them through the posting MUST NOT be relied on, because
the posting reference is cleared when the catalogue removes the row, and two cleared references
never match each other.

The public company response rate is the case that makes this normative. Its denominator survives
a removal — the employer is denormalised onto each event — while its numerator is found by
matching a reply back to its application. If that match runs through the posting, removing the
posting turns an answered application into an unanswered one and the employer is served as more
silent than it was, which is the exact distortion the ledger was introduced to remove.

#### Scenario: An answered application stays answered after its posting is removed

- **WHEN** an employer replied to an application and the posting is later deleted from the
  catalogue
- **THEN** the rollup still counts that application as answered
- **AND** the company's served response rate is unchanged by the deletion

#### Scenario: Two applications whose postings were removed stay distinct

- **WHEN** one user holds two applications, each to a different employer, and both postings are
  deleted
- **THEN** each reply is still paired with its own application
- **AND** neither employer is credited with the other's reply

#### Scenario: Reply timing is unaffected by removal

- **WHEN** the median time to first reply is computed for a company whose postings were deleted
- **THEN** it is the same value it was before the deletion

### Requirement: Mail links to the application

The system SHALL attach a classified email to the application it concerns rather than to the
catalogue posting, so a linked thread cannot be orphaned by a deletion. Every existing behaviour
built on that link — automatic linking by a deterministic match, a suggestion the user confirms,
monotonic stage advance, and recording an application from a piece of mail — MUST continue to
behave identically.

#### Scenario: Linked mail follows the application

- **WHEN** an email is linked to an application whose posting is later removed
- **THEN** the email remains linked to that application

#### Scenario: Stage advance is unchanged

- **WHEN** a linked email carries a signal that maps strictly forward from the application's
  current stage, at or above the confidence threshold
- **THEN** the application's stage advances exactly as before

### Requirement: Existing tracked applications are carried over intact

The migration SHALL create one application for every existing tracked application — every
`(user, job)` interaction whose `applied_at` is set — carrying over its dates, stage, notes and
follow-up mark, taking the employer and role title from the posting it points at, and keeping
that posting linked. Interactions that were never applied to MUST NOT produce an application, and
the carry-over MUST be re-runnable without creating duplicates.

Existing ledger events and existing linked mail MUST be pointed at the application the carry-over
created for them, so no fact recorded before this change loses the application it belongs to.

#### Scenario: Every applied interaction becomes an application

- **WHEN** the carry-over runs
- **THEN** each interaction with `applied_at` set yields exactly one application with the same
  dates, stage, notes and follow-up mark, linked to the same posting

#### Scenario: A viewed-but-not-applied job yields nothing

- **WHEN** the carry-over runs over an interaction that was only viewed or saved
- **THEN** no application is created for it

#### Scenario: Re-running the carry-over is safe

- **WHEN** the carry-over runs a second time
- **THEN** no duplicate applications are created

### Requirement: The board shows an application whose posting is gone

The tracking board SHALL list an application after the catalogue has removed the posting it was
made against. The posting's fields MAY be absent from the row; the employer and the role title
MUST be present regardless, read from the application itself.

Each row SHALL carry an identifier of its own, independent of any posting. The board has always
been a list of applications and only borrowed the posting's slug because one was always at hand;
an application with no posting has no slug to borrow, and a card the interface cannot address is
a card it cannot open, route to, or key a list by.

#### Scenario: A pruned posting leaves the card standing

- **WHEN** the catalogue removes a posting a user applied to
- **AND** that user loads their tracking board
- **THEN** the application is listed, showing the employer and role title it carries
- **AND** the row reports no posting

#### Scenario: The row is addressable without a posting

- **WHEN** a row carries no posting
- **THEN** it still carries an identifier the interface can open and route to

#### Scenario: A row with a posting is unchanged

- **WHEN** a row's posting is still in the catalogue
- **THEN** the posting's fields are present exactly as before
