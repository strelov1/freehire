## MODIFIED Requirements

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
