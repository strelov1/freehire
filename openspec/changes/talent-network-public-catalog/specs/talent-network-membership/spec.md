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
- **AND** the account appears in the catalogue under the public projection, with its name
  and employers withheld

#### Scenario: The database refuses the retired value

- **WHEN** any statement attempts to store `public`
- **THEN** the constraint rejects it

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
