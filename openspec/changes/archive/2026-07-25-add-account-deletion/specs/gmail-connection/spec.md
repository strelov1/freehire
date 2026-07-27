## ADDED Requirements

### Requirement: Grant revoked when the account is erased

Deleting the owning account SHALL revoke the Gmail grant at Google, not merely
discard the stored token, so the system genuinely loses mailbox access.

- Revocation SHALL run before the stored token is deleted, since the token is
  the only way to revoke.
- Revocation SHALL be best-effort: a failed or timed-out revoke SHALL be logged
  and SHALL NOT block the account deletion, because the discarded token leaves
  this system unable to use the grant regardless.

#### Scenario: Account deletion revokes the grant

- **WHEN** a member with a connected Gmail account deletes their account
- **THEN** the refresh token is revoked at Google before it is discarded

#### Scenario: Revoke failure does not block deletion

- **WHEN** the revoke call to Google fails while deleting an account
- **THEN** the failure is logged and the account deletion still completes
