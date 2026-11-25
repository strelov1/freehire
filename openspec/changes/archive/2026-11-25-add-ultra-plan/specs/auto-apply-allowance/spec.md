## ADDED Requirements

### Requirement: auto-apply draws on a daily allowance

Starting an auto-apply SHALL consume one unit of the account's daily auto-apply allowance.
The allowance SHALL be zero on free, a small daily count on pro, and unlimited on ultra.

#### Scenario: A free account

- **WHEN** an account on free asks to auto-apply
- **THEN** the request is refused with 402, as it is today

#### Scenario: A pro account within its allowance

- **WHEN** an account on pro that has started fewer auto-applies today than its allowance
  asks to auto-apply
- **THEN** the request succeeds and one unit is spent

#### Scenario: A pro account at its allowance

- **WHEN** an account on pro that has spent its daily allowance asks to auto-apply
- **THEN** the request is refused with 402, and the refusal carries what was used and what
  was allowed

#### Scenario: An ultra account

- **WHEN** an account on ultra asks to auto-apply, however many it has already started today
- **THEN** the request succeeds

#### Scenario: The day rolls over

- **WHEN** a pro account that spent its allowance yesterday asks again after midnight UTC
- **THEN** the request succeeds

### Requirement: This feature enforces from its first deploy

The auto-apply allowance SHALL refuse, not merely count, from the moment it ships — unlike
every other metered feature, which ships counting-only until enforcement is switched on.

#### Scenario: Enforcement is not switched on for other features

- **WHEN** the deployment has enforcement disabled for the features that ship in shadow
- **THEN** an over-allowance auto-apply is still refused

### Requirement: The allowance is spent on work actually queued

One unit SHALL be spent when a request creates a new queue entry, and SHALL NOT be spent
again for a repeat request naming the same posting.

#### Scenario: The same posting twice

- **WHEN** an account asks to auto-apply to a posting it has already queued
- **THEN** the second request spends nothing, because a double-clicked button must not cost
  somebody an attempt

#### Scenario: A posting already applied to

- **WHEN** an account asks to auto-apply to a posting it has already applied to
- **THEN** the request is refused and nothing is spent

#### Scenario: A request refused before the queue

- **WHEN** a request is refused because the account has no CV, or the posting is on an
  unsupported platform
- **THEN** nothing is spent

### Requirement: The refusal says what to do about it

A refusal for a spent allowance SHALL name the auto-apply feature and carry the day's
figures, so a surface can render the same numbers a success reports.

#### Scenario: A pro account is refused

- **WHEN** an account on pro is refused for a spent allowance
- **THEN** the response names auto-apply and carries how many were used and how many are
  allowed
