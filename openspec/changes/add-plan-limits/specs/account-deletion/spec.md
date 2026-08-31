## MODIFIED Requirements

### Requirement: Erasure covers every user-owned record

Deleting an account SHALL remove every record the member owns, across their
identity, credentials, activity, generated artifacts, and mail.

- Identity and access: the `users` row, external sign-in identities, API keys.
- Activity: job interactions (viewed / saved / applied / dismissed / tracked and
  their stages), votes, reminders and reminder settings, subscriptions and their
  matches, saved searches, search profiles, dismissals, swipe state.
- Generated artifacts: CVs, tailoring sessions, per-job match analyses, ATS
  analysis, the stored CV and its derived structured form and embedding.
- Economy: the plan record on the `users` row and every consumption-ledger entry.
- Mail: the hosted mailbox address, the Gmail connection, and every stored
  message from either source.
- Community: the member's persona (their pseudonymous handle).
- Contributions: link contributions and job submissions they authored.

#### Scenario: No user-owned row survives

- **WHEN** an account with records across job tracking, CVs, allowance consumption, saved searches, mail, and community is deleted
- **THEN** no row keyed to that user id remains in any of those tables

#### Scenario: The email address is released

- **WHEN** a deleted member's email address is used to register a new account
- **THEN** registration succeeds and the new account starts empty, sharing nothing with the deleted one

#### Scenario: A deleted account's consumption does not follow a new one

- **WHEN** an account that had exhausted a daily allowance is deleted and a new account is
  registered on the same address
- **THEN** the new account starts on the free plan with its full daily allowances

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
- **THEN** it states that deletion is permanent and unrecoverable and lists what will be erased, including their CV, mail, analyses, and their plan

#### Scenario: Confirmation gates the action

- **WHEN** the typed confirmation does not match the member's email
- **THEN** the delete action stays disabled

#### Scenario: After deletion the client is signed out

- **WHEN** the deletion request succeeds
- **THEN** the client drops its session state and lands on a public page as a signed-out visitor

#### Scenario: A subscription in force is named before deletion

- **WHEN** a member whose `pro_until` is in the future opens the delete-account surface
- **THEN** the surface states that deleting the account does not cancel a subscription held
  by the payment provider, and that they must cancel it there
