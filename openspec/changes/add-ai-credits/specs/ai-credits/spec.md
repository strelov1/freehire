## ADDED Requirements

### Requirement: Unified per-user points balance

The system SHALL maintain a single points balance per user that both AI features
(match and tailor) draw from. The balance for the current period SHALL equal the
period's granted points minus the sum of debits recorded for that user in the
same period. The balance MUST be persisted in a materialized `credit_balances`
row (one per user) carrying the current `period`, `remaining`, and `updated_at`,
kept consistent with the ledger on every debit.

#### Scenario: Fresh user receives the monthly grant

- **WHEN** a user with no prior credit activity first triggers a metered action
- **THEN** the system grants the configured monthly amount for the current period and the balance reflects that grant minus the action's cost

#### Scenario: Balance derives from the ledger

- **WHEN** the `credit_balances` row is recomputed from `credit_ledger` for a user's current period
- **THEN** `remaining` equals the period grant minus the sum of that period's debits

### Requirement: Monthly grant with lazy reset

Points SHALL be granted once per calendar month (UTC, keyed `YYYY-MM`) in the
configured amount (`CREDITS_MONTHLY_GRANT`, default 20). Unused points from a
prior period MUST NOT roll over — the grant is use-it-or-lose-it. The grant MUST
be applied lazily on the first access in a new period (no scheduled job), and a
given user MUST receive at most one grant per period.

#### Scenario: New period resets the balance

- **WHEN** a user whose stored balance period is an earlier month triggers a metered action in the current month
- **THEN** the system resets the balance to the monthly grant for the current period before applying the debit, discarding any unused prior-period points

#### Scenario: Grant is idempotent within a period

- **WHEN** a user triggers multiple metered actions within the same period
- **THEN** the monthly grant is recorded exactly once for that `(user, period)` and later actions only debit

### Requirement: Append-only credit ledger

The system SHALL record every grant and debit as an immutable row in
`credit_ledger` carrying `user_id`, `period`, `kind` (`grant` or `debit`),
`feature` (`match` or `tailor`; null for grants), signed `delta`, an optional
`ref`, and `created_at`. The ledger is the source of truth; balances are derived
from it. The schema MUST accommodate a future `kind = 'purchase'` grant without
migration of existing rows.

#### Scenario: Debit is recorded with its feature and ref

- **WHEN** a metered action debits points
- **THEN** a `debit` row is appended with the feature, the negative delta equal to the action cost, and the action's `ref`

#### Scenario: Grant is recorded

- **WHEN** the monthly grant is applied for a user's period
- **THEN** a `grant` row is appended with a positive delta equal to the configured monthly amount

### Requirement: Atomic idempotent debit

Debiting points SHALL be atomic and idempotent by `(user, feature, ref)`. A
debit MUST first apply any pending lazy period reset, then succeed only if the
remaining balance is at least the action cost, decrement the materialized balance
and append the ledger row in the same transaction (serialized per user via row
lock), and MUST NOT debit twice for the same `(user, feature, ref)`. Costs are
configurable per action (`CREDITS_COST_MATCH` default 1, `CREDITS_COST_TAILOR`
default 3).

#### Scenario: Sufficient balance debits once

- **WHEN** a user with enough remaining points performs a metered action for a new `ref`
- **THEN** the balance drops by the action cost and one debit row is appended

#### Scenario: Repeat action for the same ref is free

- **WHEN** a user performs the same metered action for a `(feature, ref)` already debited
- **THEN** no additional points are consumed and no new debit row is appended

#### Scenario: Insufficient balance is rejected

- **WHEN** a user whose remaining points are less than the action cost attempts a metered action for a new `ref`
- **THEN** the debit fails, the balance is unchanged, and no ledger row is appended

#### Scenario: Concurrent debits do not oversell

- **WHEN** two debits for the same user race with only enough points for one
- **THEN** exactly one succeeds and the other is rejected for insufficient balance

### Requirement: Balance and insufficient-points contract

Callers SHALL be able to read the current `remaining` and the period `resets_at`
without consuming points. When a metered action cannot proceed because the
balance is below the action cost, the system SHALL respond `HTTP 402` with a body
carrying `error`, `remaining`, and `resets_at`.

#### Scenario: Read exposes remaining and reset date

- **WHEN** a signed-in caller reads a balance-bearing endpoint
- **THEN** the response includes the current `remaining` points and the `resets_at` date for the current period, with no points consumed

#### Scenario: Out of points returns 402

- **WHEN** a metered action is attempted with insufficient remaining points
- **THEN** the system responds `402` with `error`, `remaining`, and `resets_at`, and does not invoke the LLM
