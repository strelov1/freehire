## ADDED Requirements

### Requirement: Membership is a single opt-in with two states

A candidate SHALL be either a member of the Talent Network or not. The system SHALL NOT
offer the candidate a choice of how much of themselves the public tier shows: the public
projection is fixed, and joining is the only decision.

`users.talent_network_visibility` SHALL admit exactly two values, `off` and `anonymous`.
`off` SHALL be the default for every account.

#### Scenario: An account that has never opted in

- **WHEN** an account is created, or an existing account has never touched the setting
- **THEN** `talent_network_visibility` is `off`
- **AND** the account appears in no catalogue response and resolves at no public card URL

#### Scenario: Joining

- **WHEN** a signed-in candidate turns the toggle on
- **THEN** the stored value becomes `anonymous`
- **AND** the response echoes the stored value rather than the requested one

#### Scenario: Leaving

- **WHEN** a member turns the toggle off
- **THEN** the stored value becomes `off`
- **AND** their card URL answers 404 on the next request, without waiting for any cache to
  expire

#### Scenario: A value outside the two states is refused

- **WHEN** a request asks to set any value other than `off` or `anonymous`
- **THEN** the request is refused with 400 and the stored value is unchanged

### Requirement: The retired third state resolves to membership

The `public` state SHALL be retired. Every row holding it SHALL be rewritten to
`anonymous`, and the database SHALL reject the value thereafter.

Rewriting to `anonymous` rather than `off` is deliberate: the rewrite may only ever narrow
what is disclosed about a person, never remove them from a network they chose to be in.

#### Scenario: An account that had chosen the retired state

- **WHEN** the migration runs against an account whose value is `public`
- **THEN** the account's value becomes `anonymous`

#### Scenario: A member who joined before handles existed

- **WHEN** an account is a member and has no handle — because it joined before the column
  existed, or because the migration rewrote it from the retired state
- **THEN** it is absent from the catalogue and its card answers 404 until a handle is
  minted, since a member with no address is one the catalogue cannot link to
- **AND** a one-off backfill mints one for every such account, after which they appear
  under the public projection
- **AND** the backfill is idempotent: a re-run mints nothing

#### Scenario: The database refuses the retired value

- **WHEN** any statement attempts to store `public`
- **THEN** the constraint rejects it

### Requirement: A member has one permanent catalogue handle

The first time a candidate joins, the system SHALL mint them a catalogue handle and store
it. The handle SHALL be the only public identifier of their card, SHALL be unique across
all accounts, and SHALL never change once minted.

The handle SHALL NOT be the account's `username`, and SHALL NOT be derived from the
candidate's name, email address or any contact detail. It SHALL be derived from the
professional category their most recent role title resolves to, plus a random suffix, and
the derived part SHALL be frozen at mint time.

An account that has never joined SHALL have no handle.

#### Scenario: The handle is minted on first join

- **WHEN** a candidate who has never been a member turns the toggle on
- **THEN** a handle is stored for them
- **AND** their card is reachable at that handle

#### Scenario: The handle survives leaving and rejoining

- **WHEN** a member leaves the network and later rejoins
- **THEN** their handle is the same one they had before
- **AND** a link shared before they left resolves again

#### Scenario: The handle survives a change of job

- **WHEN** a member uploads a new CV whose most recent role resolves to a different
  category
- **THEN** their handle is unchanged

#### Scenario: The handle is not the account username

- **WHEN** an account's username was derived from their email's local part
- **THEN** the minted handle does not contain that username
- **AND** the account's username appears nowhere in any public catalogue response

#### Scenario: Two candidates whose titles resolve alike

- **WHEN** two candidates in the same category join
- **THEN** each is minted a distinct handle
- **AND** neither mint fails

#### Scenario: A title that resolves to no category

- **WHEN** a candidate joins and their most recent title resolves to no category
- **THEN** a handle is still minted, on a neutral base

### Requirement: The member's own surfaces say whether a visitor can see them

Membership and being VISIBLE are different facts, and they come apart in the most
ordinary way there is: a candidate joins before uploading a CV. They are a member, they
hold a handle, and the catalogue's stamp gate still excludes them — so their card 404s.

Every surface that reports a candidate's Talent Network standing SHALL read whether they
are LISTED, not whether they are a member. It SHALL NOT offer a link to their public card
unless that card exists.

#### Scenario: A member with no readable CV

- **WHEN** a candidate joins before uploading a CV, or while its extraction is still
  running
- **THEN** their standing reads as in the network but not yet shown, and says what is
  missing
- **AND** no link to their public card is offered, because it would answer 404

#### Scenario: A member whose CV is readable

- **WHEN** a member's structured extract matches the CV on file
- **THEN** their standing reads as appearing in the catalogue
- **AND** a link to their own public card is offered

### Requirement: The control is reachable without a direct URL

The Talent Network SHALL be reachable from the account navigation, and the account's
profile page SHALL carry an invitation to join for a candidate who is not a member.

This is a requirement and not a nicety: a control reachable only by typing its path is
indistinguishable from a control that does not exist, and the catalogue's population
depends entirely on this.

#### Scenario: A signed-in candidate opens their account

- **WHEN** any signed-in candidate views a page under `/my`
- **THEN** the account navigation lists a Talent Network section
- **AND** following it opens the membership control

#### Scenario: A non-member opens their profile

- **WHEN** a candidate who is not a member opens `/my/profile`
- **THEN** the page shows an invitation stating what joining publishes and what it
  withholds
- **AND** the invitation leads to the membership control

#### Scenario: A member opens their profile

- **WHEN** a member opens `/my/profile`
- **THEN** the page states that they are in the Talent Network
- **AND** offers to open their own public card as a visitor would see it
