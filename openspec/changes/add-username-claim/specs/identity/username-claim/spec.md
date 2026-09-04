## Purpose

Gives every account a single, account-level username: a short, unique, user-visible name that other features (the hosted mailbox, and later the talent-network public profile) adopt instead of inventing their own naming scheme.

## ADDED Requirements

### Requirement: Username format

A username SHALL be 3-30 characters, lowercase letters and digits with single internal hyphens only, and MUST start and end with a letter or digit (no leading, trailing, or consecutive hyphens).

#### Scenario: Valid username accepted

- **WHEN** a username matching the format (e.g. `ivan-petrov`, `john123`) is submitted
- **THEN** it passes format validation

#### Scenario: Invalid format rejected

- **WHEN** a submitted username is too short, too long, contains uppercase letters or characters outside `[a-z0-9-]`, or starts/ends/doubles a hyphen
- **THEN** it is rejected with a format error

### Requirement: Reserved names are unavailable

The system SHALL maintain a reserved-name list (service/role addresses, automation names, and product-identity terms) that no user may claim, regardless of whether the name is otherwise unclaimed.

#### Scenario: Reserved name rejected

- **WHEN** a user attempts to claim a name on the reserved list
- **THEN** the claim is rejected as unavailable, even though no other account holds it

### Requirement: Read the caller's own username

The system SHALL let a signed-in user read their own account's current username (null if none) and, when set, the time of its last explicit change.

#### Scenario: Reading before any username exists

- **WHEN** a signed-in user with no username reads their own state
- **THEN** the response reports no username

#### Scenario: Reading after a claim

- **WHEN** a signed-in user who has claimed a username reads their own state
- **THEN** the response reports that username

### Requirement: Username availability check

The system SHALL expose a check that reports whether a candidate username is available: valid format, not reserved, and not already claimed by another account.

#### Scenario: Available name

- **WHEN** the caller checks a name that is valid, unreserved, and unclaimed
- **THEN** the check reports it available

#### Scenario: Unavailable name

- **WHEN** the caller checks a name that is invalid, reserved, or already claimed
- **THEN** the check reports it unavailable

### Requirement: Default username on first need

The system SHALL lazily allocate a default username for an account the first time one is actually needed (e.g. a feature that depends on the account having a username), deriving a candidate from the account's email and appending the smallest free numeric suffix on collision. Allocation MUST be idempotent — an account that already has a username is returned its existing one, never re-allocated.

#### Scenario: First need allocates a default

- **WHEN** an account with no username reaches a point where one is needed
- **THEN** the system derives a candidate from the account's email, resolves any collision with a numeric suffix, stores it, and returns it

#### Scenario: Repeated need is idempotent

- **WHEN** an account that already has a username reaches a point where one is needed again
- **THEN** the same existing username is returned and no reallocation occurs

### Requirement: Explicit username claim

The system SHALL let a signed-in user claim a specific desired username, replacing any username the account currently has. A desired name that fails format validation, is reserved, or is already claimed by another account MUST be rejected outright — the system SHALL NOT substitute a suffixed alternative.

#### Scenario: Successful claim

- **WHEN** a signed-in user claims a desired username that is valid, unreserved, and unclaimed
- **THEN** the account's username becomes that name

#### Scenario: Claiming a taken name is rejected, not suffixed

- **WHEN** a signed-in user claims a desired username already held by another account
- **THEN** the claim is rejected as taken and no suffixed variant is allocated

#### Scenario: Claiming an invalid or reserved name is rejected

- **WHEN** a signed-in user claims a desired username that fails format validation or is reserved
- **THEN** the claim is rejected and the account's username is unchanged

### Requirement: Username change is rate-limited

The system SHALL let a user change their username to a different desired value, but SHALL NOT allow more than one such change per rolling 30-day period since their last successful explicit claim. The account's very first explicit claim is never subject to this limit.

#### Scenario: Change within the cooldown is rejected

- **WHEN** a user who changed their username less than 30 days ago attempts another change
- **THEN** the change is rejected until the cooldown elapses

#### Scenario: First claim is never rate-limited

- **WHEN** an account with no prior claim claims a username for the first time
- **THEN** the claim succeeds regardless of any cooldown state

#### Scenario: Change after the cooldown succeeds

- **WHEN** a user whose last change was 30 or more days ago claims a new available username
- **THEN** the change succeeds and the cooldown restarts from this change

### Requirement: A changed-away username becomes reclaimable

When a user changes their username to a new value, the previous username SHALL become available for any other account to claim.

#### Scenario: Released name is claimable by another account

- **WHEN** a user changes away from username A to username B
- **THEN** a different account can subsequently claim username A
