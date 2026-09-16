# identity-confirmation Specification

## Purpose
TBD - created by archiving change consistent-identity-confirmation. Update Purpose after archive.
## Requirements
### Requirement: Actions gated on recent credential control

Certain account actions SHALL require, in addition to a valid session, a short-lived proof
that the member still controls the account's credentials. A session cookie alone SHALL NOT
authorize them, because a hijacked session must not be able to mint a permanent credential,
change the password, or destroy the account.

The gated actions are creating an API key, revoking an API key, changing the password, and
deleting the account. An attempt without a valid proof SHALL be refused with HTTP `428` and a
machine-readable code distinguishing it from an ordinary authentication failure.

The proof SHALL be bound to the exact session that obtained it, so a proof cannot be moved to
another session, and SHALL expire after a bounded lifetime.

#### Scenario: A gated action without a proof is refused distinguishably

- **WHEN** a signed-in member requests a gated action carrying a valid session cookie and no
  valid recent-auth proof
- **THEN** the system responds `428` with the code `recent_auth_required`
- **AND** the action is not performed

#### Scenario: A proof authorizes the action

- **WHEN** the same request carries a valid, unexpired proof bound to that session
- **THEN** the action is performed

### Requirement: Every gated action offers a way to obtain the proof

A client surface that can trigger a gated action SHALL, on that same surface, offer every way
the member can actually produce a proof. A surface SHALL NOT state a requirement it gives the
member no way to satisfy.

- An account that has a password SHALL be offered a password input on the surface that
  performs the action.
- An account with no password SHALL be offered its connected sign-in providers, listing only
  providers whose identity is active.
- Where the action is performed inside a modal dialog, the means of confirmation SHALL be
  inside that dialog, because a modal makes the rest of the page unreachable.

#### Scenario: A modal action carries its own confirmation

- **WHEN** a member opens a dialog that performs a gated action
- **THEN** the means of confirmation is inside that dialog and reachable while it is open

#### Scenario: A password account is offered a password

- **WHEN** a member whose account has a password opens a surface performing a gated action
- **THEN** that surface offers a password input

#### Scenario: A provider-only account is offered its providers

- **WHEN** a member whose account has no password opens a surface performing a gated action
- **THEN** that surface offers one confirmation control per connected provider whose identity
  is active, and no password input

#### Scenario: No confirmation is demanded without a way forward

- **WHEN** a surface reports that confirmation is required
- **THEN** the same surface either renders a control by which the member can provide it, or
  states why none can be offered and what to do instead — never a demand with nothing beside
  it

#### Scenario: A failure to offer the methods is not a dead end

- **WHEN** a surface cannot load the member's sign-in providers
- **THEN** it says so and offers to try again, rather than leaving a state nothing can move

### Requirement: A pending action survives the provider round trip

Confirming through an OAuth provider leaves the site and returns by a full-page navigation,
which destroys unsaved client state. The client SHALL carry the pending action across that
navigation and resume it on return, so the member does not silently lose the work that
triggered the confirmation.

- On return the client SHALL restore the surface that was open and the non-sensitive input the
  member had already supplied.
- A secret or an anti-mistake confirmation — a password, or a typed-address confirmation whose
  purpose is to slow the member down — SHALL NOT be carried across the navigation.
- The completed action SHALL NOT be performed automatically on return; the member SHALL
  perform it deliberately.
- The carried state SHALL be discarded once consumed, so a later unrelated visit does not
  reopen it.

#### Scenario: The pending surface reopens with its input intact

- **WHEN** a member fills in a gated surface, confirms through a provider, and is returned
- **THEN** the same surface is open again with the non-sensitive input restored

#### Scenario: A secret is not carried across the round trip

- **WHEN** the client stores the pending action before leaving for the provider
- **THEN** no password and no typed-address confirmation is included in what it stores

#### Scenario: Returning does not perform the action

- **WHEN** a member returns from the provider with a valid proof
- **THEN** the action has not been performed and awaits a deliberate confirmation

#### Scenario: Carried state is consumed once

- **WHEN** a member returns from the provider and the surface is restored
- **THEN** a subsequent unrelated visit to the same page does not restore it again

### Requirement: The member is told the confirmation state

The client SHALL tell the member where they stand: that confirmation is needed before the
action, and that it succeeded after it. A returning member SHALL NOT have to guess whether the
round trip worked.

The client's view of the proof is an indication, never the authority: the proof is held in a
cookie the client cannot read. When the server refuses a supposedly confirmed action with
`428`, the client SHALL drop its indication and ask for confirmation again rather than repeat
a generic failure.

#### Scenario: Confirmation is presented as a step, not as an error

- **WHEN** a member opens a surface that performs a gated action
- **THEN** the confirmation is presented as part of that action before it is attempted, not
  only as an error after the server refuses

#### Scenario: A returning member is told it worked

- **WHEN** a member returns from a successful provider confirmation
- **THEN** the surface states that identity was confirmed

#### Scenario: The server overrules the client's indication

- **WHEN** the client believes a proof is held but the server answers a gated action with `428`
- **THEN** the client presents the confirmation step again rather than a generic failure

#### Scenario: Every gated surface handles 428

- **WHEN** any surface performing a gated action receives `428`
- **THEN** it presents the confirmation step, or — where the confirmation the member could
  offer has already been given and refused — names what would actually resolve it
- **AND** never reports it as an unspecified error

#### Scenario: A blank answer is not sent as if it were one

- **WHEN** a member submits a gated action without supplying the confirmation asked of them
- **THEN** nothing is sent to the server, and the surface says what is missing
- **AND** the member is never told their answer was wrong when they gave none

