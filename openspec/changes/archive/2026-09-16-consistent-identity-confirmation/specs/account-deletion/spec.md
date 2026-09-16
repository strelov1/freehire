## MODIFIED Requirements

### Requirement: Deletion surface states the consequences

The account settings area SHALL offer account deletion behind a surface that
states plainly, before confirmation, that deletion is permanent, that nothing can
be restored, and what is erased.

- The surface SHALL require the member to type their own email address to enable
  the destructive action; the action SHALL be disabled until it matches.
- After a successful deletion the client SHALL clear its local session state and
  redirect to the public site.
- Deletion is gated on a proof of recent credential control, so the surface SHALL offer the
  member a way to produce that proof from within the surface itself: a password input for an
  account that has a password, and its connected sign-in providers for an account that does
  not.
- Confirming through a provider leaves the site and returns by a full-page navigation, which
  closes the surface. The client SHALL reopen the surface on return and state that identity was
  confirmed.
- The typed email confirmation SHALL NOT be restored on return. It exists to slow a member
  down before an irreversible act, so it SHALL be re-entered rather than carried across the
  navigation.

#### Scenario: Member sees what deletion means

- **WHEN** a signed-in member opens the delete-account surface
- **THEN** it states that deletion is permanent and unrecoverable and lists what will be erased, including their CV, mail, analyses, and credits

#### Scenario: Confirmation gates the action

- **WHEN** the typed confirmation does not match the member's email
- **THEN** the delete action stays disabled

#### Scenario: After deletion the client is signed out

- **WHEN** the deletion request succeeds
- **THEN** the client drops its session state and lands on a public page as a signed-out visitor

#### Scenario: The surface can produce the proof it needs

- **WHEN** a member opens the delete-account surface
- **THEN** the surface offers a password input if the account has a password, or a
  confirmation control per active connected provider if it does not

#### Scenario: The surface reopens after a provider round trip

- **WHEN** a member confirms through a provider from the delete-account surface and is returned
- **THEN** the surface is open again and states that identity was confirmed

#### Scenario: The typed address is re-entered after the round trip

- **WHEN** the delete-account surface reopens after a provider round trip
- **THEN** the typed email confirmation is empty and the delete action is disabled until it is
  typed again
