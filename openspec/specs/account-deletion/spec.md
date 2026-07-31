# account-deletion Specification

## Purpose

Self-serve, irreversible erasure of a member's account: what is erased across
Postgres, object storage and Google, what deliberately survives, and the surface
that fronts it.

## Requirements

### Requirement: Self-serve irreversible account deletion

The system SHALL expose `DELETE /api/v1/me`, which permanently erases the calling
member's account. The endpoint SHALL authenticate by session cookie only and MUST
NOT be reachable with an API key, so a leaked programmatic credential cannot
destroy its owner's account.

- Deletion SHALL be immediate and irreversible: no soft-delete flag, no grace
  period, no restore path.
- The request body SHALL carry the member's own email address as confirmation.
  The comparison SHALL be case-insensitive, matching how the account's email is
  stored and looked up at login.
- On success the system SHALL respond `204 No Content` and expire the session
  cookie in the same response.

#### Scenario: Member deletes their own account

- **WHEN** a signed-in member calls `DELETE /api/v1/me` with a body confirming their own email address
- **THEN** the system erases the account, responds `204`, and returns a `Set-Cookie` that expires the session cookie

#### Scenario: Confirmation does not match

- **WHEN** a signed-in member calls `DELETE /api/v1/me` with a confirmation email that is not their own (or with no confirmation at all)
- **THEN** the system responds `400`, erases nothing, and leaves the session intact

#### Scenario: Confirmation differs only in letter case

- **WHEN** a signed-in member confirms with their own email typed in a different letter case than stored
- **THEN** the system accepts the confirmation and proceeds with deletion

#### Scenario: API key cannot delete an account

- **WHEN** a client calls `DELETE /api/v1/me` with a valid `Authorization: Bearer <api-key>` and no session cookie
- **THEN** the system responds `401` and erases nothing

#### Scenario: Anonymous caller

- **WHEN** an unauthenticated client calls `DELETE /api/v1/me`
- **THEN** the system responds `401`

### Requirement: Erasure covers every user-owned record

Deleting an account SHALL remove every record the member owns, across their
identity, credentials, activity, generated artifacts, and mail.

- Identity and access: the `users` row, external sign-in identities, API keys.
- Activity: job interactions (viewed / saved / applied / dismissed / tracked and
  their stages), votes, reminders and reminder settings, subscriptions and their
  matches, saved searches, search profiles, dismissals, swipe state.
- Generated artifacts: CVs, tailoring sessions, per-job match analyses, ATS
  analysis, the stored CV and its derived structured form and embedding.
- Economy: the credit balance and every credit-ledger entry.
- Mail: the hosted mailbox address, the Gmail connection, and every stored
  message from either source.
- Community: the member's persona (their pseudonymous handle).
- Contributions: link contributions and job submissions they authored.

#### Scenario: No user-owned row survives

- **WHEN** an account with records across job tracking, CVs, credits, saved searches, mail, and community is deleted
- **THEN** no row keyed to that user id remains in any of those tables

#### Scenario: The email address is released

- **WHEN** a deleted member's email address is used to register a new account
- **THEN** registration succeeds and the new account starts empty, sharing nothing with the deleted one

### Requirement: Artifacts outside Postgres are erased

Deletion SHALL erase the member's artifacts held outside the database, which no
foreign-key cascade can reach.

- Object storage: the stored CV object, the member's headshot, every
  referral-proof PDF, and the raw MIME object of every hosted email the member
  received.
- Google: the Gmail OAuth grant SHALL be revoked at Google before the stored
  token is discarded, so mailbox access is genuinely surrendered rather than
  merely forgotten.
- Object keys SHALL be collected from the database BEFORE any row is deleted,
  because the mail objects are addressable only through the rows that name them.

#### Scenario: Stored objects are gone

- **WHEN** a member with a stored CV, a headshot, a referral proof, and hosted mail is deleted
- **THEN** none of those objects remain in the bucket

#### Scenario: Google grant surrendered

- **WHEN** a member with a connected Gmail account is deleted
- **THEN** the refresh token is revoked at Google before the connection row is dropped

#### Scenario: Storage failure aborts the deletion

- **WHEN** object storage is unreachable while erasing a member's objects
- **THEN** the system responds `503`, the account and all of its rows remain intact, and retrying the deletion is safe

#### Scenario: Google unreachable does not block deletion

- **WHEN** the revoke call to Google fails or times out
- **THEN** the failure is logged and the deletion proceeds, because a stale grant with no stored token is not usable by this system

### Requirement: Deleted accounts have no live sessions

Once an account is erased, no previously issued credential SHALL grant access,
on any device.

#### Scenario: Session cookie on another device

- **WHEN** a client presents a session cookie for a deleted account that has not yet expired
- **THEN** the system responds `401` rather than serving the request or failing with a server error

#### Scenario: API key after deletion

- **WHEN** a client presents an API key that belonged to a deleted account
- **THEN** the system responds `401`

### Requirement: Records that deliberately survive

Some records SHALL outlive the account because they are not the member's own
data or because removing them would destroy other people's content.

- Moderation and authoring audit trails keep their reference nulled, never
  deleted: jobs a moderator created or updated, reviewed submissions and reports,
  decided referral offers and acted-on referral requests.
- Community threads and replies survive de-authored (see the `community-threads`
  capability), so a departing member does not delete other people's discussion.
- Aggregate counters that carry no user reference (job view counts, engagement
  rollups) are unaffected.

#### Scenario: Moderator trail keeps the record, drops the identity

- **WHEN** a moderator account that created jobs and reviewed submissions is deleted
- **THEN** those jobs, submissions, and reports remain, with their creator/reviewer reference set to null

#### Scenario: Other members' replies survive

- **WHEN** a member who opened a thread that others replied to is deleted
- **THEN** the thread and every reply remain readable

### Requirement: Deletion surface states the consequences

The account settings area SHALL offer account deletion behind a surface that
states plainly, before confirmation, that deletion is permanent, that nothing can
be restored, and what is erased.

- The surface SHALL require the member to type their own email address to enable
  the destructive action; the action SHALL be disabled until it matches.
- After a successful deletion the client SHALL clear its local session state and
  redirect to the public site.

#### Scenario: Member sees what deletion means

- **WHEN** a signed-in member opens the delete-account surface
- **THEN** it states that deletion is permanent and unrecoverable and lists what will be erased, including their CV, mail, analyses, and credits

#### Scenario: Confirmation gates the action

- **WHEN** the typed confirmation does not match the member's email
- **THEN** the delete action stays disabled

#### Scenario: After deletion the client is signed out

- **WHEN** the deletion request succeeds
- **THEN** the client drops its session state and lands on a public page as a signed-out visitor
