## ADDED Requirements

### Requirement: A signed-in user can link one Discord account

The system SHALL let an authenticated user bind their Discord account to their freehire
account through Discord's authorization-code flow, requesting only the `identify` and
`guilds.join` scopes. The obtained access token SHALL be used within the callback request
and never stored.

Both the start and the callback routes SHALL require a session cookie; an API key MUST NOT
be accepted, because a link changes what an account is rather than what it can read.

#### Scenario: Linking succeeds

- **WHEN** a signed-in user starts the flow and grants consent on Discord
- **THEN** the system stores the Discord user id against their freehire account, adds them
  to the configured guild, and returns them to `/my/integrations` showing the account as
  connected

#### Scenario: The Discord account is already linked elsewhere

- **WHEN** the callback resolves a Discord user id already bound to a different freehire
  account
- **THEN** the system refuses the link with HTTP 409 and a message naming the conflict, and
  the existing binding is left untouched

#### Scenario: An API key is presented instead of a session

- **WHEN** the link route is called with a Bearer API key and no session cookie
- **THEN** the system responds 401 and stores nothing

#### Scenario: The callback state does not match

- **WHEN** the callback arrives with a missing, expired or mismatched state cookie
- **THEN** the system refuses the exchange and stores nothing

### Requirement: The paid role tracks the subscription

The system SHALL grant the configured Discord role to a linked account whose freehire tier
is a paying tier, and SHALL revoke that role once the tier is no longer paying. Whether an
account is paying SHALL be resolved through `plan.TierOf` over `users.pro_until` and
`users.ultra_until`, never by querying a payment provider — the tier is the only answer that
accounts for Stripe, RevenueCat and granted promo time alike.

Every paying tier SHALL receive the same single role; the system does not distinguish Pro
from Ultra in Discord.

Reconciliation SHALL run on a schedule rather than only on payment events, so a missed
webhook cannot leave a lapsed subscriber with standing access.

#### Scenario: A paying linked account has no role yet

- **WHEN** reconciliation examines a linked account whose tier is paying and whose role is
  not recorded as granted
- **THEN** the system adds the role and records when it did

#### Scenario: A subscription lapses

- **WHEN** reconciliation examines a linked account whose tier is free and whose role is
  recorded as granted
- **THEN** the system removes the role and clears the record of the grant

#### Scenario: Reconciliation repeats over an unchanged account

- **WHEN** reconciliation runs twice against an account whose tier has not moved
- **THEN** the second run makes no role call to Discord and leaves the record of the grant
  unchanged

#### Scenario: The member left the guild

- **WHEN** Discord answers that the member is unknown to the guild
- **THEN** the system treats it as an absence rather than a failure: the run continues, the
  link row is kept, and the grant record is cleared

### Requirement: One Discord account serves one freehire account

The system SHALL enforce at the database level that a Discord account is bound to at most one
freehire account, so a single subscription cannot be spread across several people.

#### Scenario: A second binding is attempted

- **WHEN** a write would bind a Discord user id that another freehire account already holds
- **THEN** the database rejects it

### Requirement: Unlinking withdraws access immediately

The system SHALL let a linked user unlink from `/my/integrations`. Unlinking SHALL revoke the
role in the same request and delete the binding. It SHALL NOT remove the person from the
guild — they were invited, and eviction is a moderation act rather than a billing one.

#### Scenario: A paying user unlinks

- **WHEN** a linked, paying user unlinks
- **THEN** the role is revoked, the binding is deleted, and their guild membership remains

#### Scenario: Unlinking when Discord is unreachable

- **WHEN** the revoke call fails
- **THEN** the binding is still deleted and the failure is logged; the next reconciliation
  finds no row and the orphaned role is left for an operator, rather than the user being
  trapped in a link they cannot remove

### Requirement: The feature is absent without credentials

The system SHALL treat incomplete Discord credentials as the feature being switched off: the
link routes SHALL respond 404, the public configuration SHALL report it disabled so the SPA
omits the card, and the reconciliation worker SHALL exit successfully without opening a
database connection.

#### Scenario: No bot token configured

- **WHEN** the deployment has no Discord credentials
- **THEN** the routes 404, the integrations page shows no Discord card, and the worker is a
  successful no-op

### Requirement: Reconciliation is bounded and resumable

The reconciliation worker SHALL bound how many accounts one run processes, so a backlog
cannot turn a `Type=oneshot` unit into a run that outlives its own timer. A run that stops
early SHALL leave the remaining accounts for the next run without losing work.

#### Scenario: More linked accounts than one run allows

- **WHEN** the number of linked accounts exceeds the per-run bound
- **THEN** the run processes up to the bound, exits successfully, and the next run examines
  the accounts the previous one did not reach rather than starting from the same ones

#### Scenario: The bound is set to an unreadable value

- **WHEN** the per-run bound is set but cannot be parsed as a positive integer
- **THEN** the run fails and names the value, rather than silently falling back to a default
