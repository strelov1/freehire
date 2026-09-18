## Purpose

Lets a person prove they represent a real company on the catalogue and gain a scoped
account that may manage that company's curated profile and publish vacancies for it,
without any moderator being in the loop for the common case.

## ADDED Requirements

### Requirement: An authenticated user claims a company

The system SHALL allow an authenticated user with no existing employer account to start a
claim on a company by name. The system SHALL resolve the claim to an existing canonical
`company_slug` when one matches, and SHALL otherwise mint a new slug from the supplied name
using the same normalization the ingest pipeline uses, so a claim on an employer already in
the catalogue always lands on the same company identity as that employer's existing
postings. A user who already holds an employer account (in any status) SHALL be refused a
second claim.

#### Scenario: Claiming an existing company resolves to its canonical slug

- **WHEN** a user claims a company name that already matches a canonical company slug or one
  of its registered spelling aliases
- **THEN** the claim is recorded against that canonical slug, not a newly minted one

#### Scenario: Claiming a company not yet in the catalogue mints a new slug

- **WHEN** a user claims a company name that matches no existing company or alias
- **THEN** the claim is recorded against a freshly normalized slug for that name

#### Scenario: A user with an existing employer account cannot start a second claim

- **WHEN** a user who already has an employer account (pending, active, or revoked) attempts
  to claim a company
- **THEN** the request is refused

### Requirement: Claiming a company reserves its slug exclusively

The system SHALL allow at most one employer account per `company_slug` at any time,
enforced so that two concurrent claims on the same company can never both succeed. The
losing claim SHALL receive a clear conflict response, not a silent overwrite or a duplicate
account.

#### Scenario: A second claim on an already-claimed company is refused

- **WHEN** a company slug already has an employer account in any status
- **THEN** a new claim on that same slug is refused with a conflict response

#### Scenario: Two simultaneous claims on the same new company race safely

- **WHEN** two users submit a claim for the same not-yet-claimed company at effectively the
  same time
- **THEN** exactly one claim succeeds and the other receives a conflict response

### Requirement: A claimed company's work email is verified by a mailed code

The system SHALL require a work email address for a pending claim and SHALL verify it by
sending a time-limited numeric code to that address and requiring it back from the user.
The code SHALL expire and SHALL be rejected after a bounded number of wrong attempts,
matching the behavior already established for the account's own email-verification codes.

#### Scenario: A correct code confirms the work email

- **WHEN** a user submits the code that was mailed to their claim's work email, before it
  expires and within the attempt limit
- **THEN** the work email is recorded as confirmed for that claim

#### Scenario: An expired or exhausted code is rejected

- **WHEN** a user submits a code after it has expired, or after the wrong-attempt limit has
  been reached
- **THEN** the confirmation is refused and no claim is confirmed

### Requirement: A public webmail domain is refused as a work email

The system SHALL refuse a claim's work email when its domain is a known public/free email
provider (for example gmail.com, outlook.com, yahoo.com), before any verification code is
sent.

#### Scenario: A gmail.com address is refused outright

- **WHEN** a user submits a work email at a known public webmail domain
- **THEN** the claim step is refused and no code is sent

### Requirement: A confirmed email matching the company's known domain activates the account immediately

When a claim's work email is confirmed, the system SHALL compare its domain to the claimed
company's already-known website domain, when one is recorded. A match SHALL activate the
employer account immediately, with no further review.

#### Scenario: A matching domain activates the account at once

- **WHEN** a claim's confirmed work-email domain matches the claimed company's recorded
  website domain
- **THEN** the employer account becomes active immediately, with no moderator step

### Requirement: An unverifiable domain is routed to moderator review, not refused

When a claim's work-email domain does not match the company's recorded website, or the
company has no recorded website to compare against, the system SHALL leave the account
pending rather than activating it or refusing the claim outright. A pending claim SHALL be
visible to moderators for manual review.

#### Scenario: An unknown company website leaves the claim pending

- **WHEN** a claim's work email is confirmed for a company with no recorded website
- **THEN** the employer account stays pending and appears in the moderator review queue

#### Scenario: A mismatched domain leaves the claim pending rather than refusing it

- **WHEN** a claim's confirmed work-email domain does not match the claimed company's
  recorded website domain
- **THEN** the employer account stays pending and appears in the moderator review queue,
  and the claim is not refused

### Requirement: A moderator can approve or reject a pending claim

The system SHALL let a moderator list pending employer-account claims and approve or reject
each one. Approval SHALL activate the account and, when the company's `company_info` website
was empty, SHALL also record the claim's confirmed work-email domain there — a moderator
approving a claim the domain check could not itself verify is exactly the human vouching the
automatic path was missing, so this is where that gap closes, not the automatic path (which
never activates a claim whose website was unknown or mismatched in the first place — see "An
unverifiable domain is routed to moderator review, not refused"). Rejection SHALL remove the
claim and free its company slug for a future claim.

#### Scenario: Moderator approval activates a pending claim

- **WHEN** a moderator approves a pending employer-account claim
- **THEN** the account becomes active

#### Scenario: Moderator approval fills a previously unknown company website

- **WHEN** a moderator approves a pending claim for a company with no previously recorded
  website
- **THEN** the company's website is set to the confirmed work-email domain

#### Scenario: Moderator rejection frees the company slug

- **WHEN** a moderator rejects a pending employer-account claim
- **THEN** the claim is removed and the same company slug can be claimed again

### Requirement: Only an active employer account may act on its company

The system SHALL restrict every employer-facing capability — editing the company's curated
profile and publishing, editing, or closing a vacancy — to a `user_id`/`company_slug` pair
whose employer account status is active. A pending or revoked account SHALL be refused on
every such action.

#### Scenario: A pending account cannot publish a vacancy or edit the company profile

- **WHEN** a user whose employer account is still pending attempts to publish a vacancy or
  edit their company's profile
- **THEN** the request is refused

#### Scenario: A revoked account loses employer capabilities

- **WHEN** a user whose employer account has been revoked attempts any employer-facing
  action
- **THEN** the request is refused

### Requirement: A user may always read their own employer account's status

Unlike every capability in the previous requirement, reading the caller's own account —
its status and, once active, the company's curated profile — SHALL NOT require the account
to be active. A user with a pending or revoked account SHALL still be able to see that
status; a user with no employer account at all SHALL get a not-found response, never a
refused-but-you-have-one response that would indistinguishably describe both cases.

#### Scenario: A pending account can read its own status

- **WHEN** a user whose employer account is still pending requests their own account
- **THEN** the system returns it, showing `pending`, rather than refusing the read

#### Scenario: No employer account is a not-found, not a refusal

- **WHEN** a user with no employer account at all requests their own account
- **THEN** the system reports it as not found

### Requirement: An admin can revoke an employer account

The system SHALL let an admin revoke an active or pending employer account. A revoked
account's company slug SHALL remain reserved — it SHALL NOT become claimable again through
the ordinary self-service claim flow.

#### Scenario: Revocation blocks further employer actions

- **WHEN** an admin revokes an employer account
- **THEN** that account can no longer publish, edit, or close vacancies, or edit the company
  profile

#### Scenario: A revoked account's slug is not self-service claimable

- **WHEN** a user attempts to claim a company whose slug belongs to a revoked employer
  account
- **THEN** the claim is refused
