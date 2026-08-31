## MODIFIED Requirements

### Requirement: A caller can see what their own account did

`GET /api/v1/me/usage` SHALL report the authenticated caller's own AI activity for the
current period — model calls, failures and tokens. It MUST report only theirs, MUST answer
a caller with no activity as zeroes rather than as an error or a 404, and MUST NOT fail the
request when the gateway is unreachable.

It SHALL NOT report cost in any currency. The gateway prices from a list against a mixed
upstream pool, so the figure is neither what the operator pays nor what the caller pays —
what the caller spends is a plan allowance, reported over this same period. Two numbers in
two currencies for one thing would leave the fictional one indistinguishable from the real
one.

The period SHALL be the UTC calendar day the plan allowances already reset on, so an
allowance and an activity count are never reported against different periods. This is a
narrowing: the period was the calendar month the points balance reset on, and the balance
no longer exists.

#### Scenario: A caller with activity

- **WHEN** a caller who has used AI this period reads the endpoint
- **THEN** the response carries their call count, failures, tokens, and when the period resets

#### Scenario: The response carries no money

- **WHEN** any caller reads the endpoint
- **THEN** no field of the response states a cost, in any currency

#### Scenario: A caller with none

- **WHEN** a caller who has never used AI reads the endpoint
- **THEN** the response is `200` with zeroes

#### Scenario: Activity is owner-scoped

- **WHEN** two accounts have both used AI
- **THEN** each sees only its own

#### Scenario: The endpoint requires authentication

- **WHEN** the request carries no accepted credential
- **THEN** the response is `401`

#### Scenario: The gateway is unreachable

- **WHEN** the gateway cannot be read
- **THEN** the response is `200` with zeroes rather than an error

#### Scenario: Activity and allowance report the same period

- **WHEN** a caller reads both their usage and their plan allowances
- **THEN** both report against the same UTC day and the same reset instant

#### Scenario: The reader is shown it only in beta

- **WHEN** an account that is not a beta tester opens the usage page
- **THEN** the activity panel is not rendered, and the plan and today's consumption are
  unchanged
