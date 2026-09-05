## ADDED Requirements

### Requirement: A promo code is data, never source

Every promo code, its discount, its seat limit and its expiry SHALL live in the
`promo_codes` table. The repository SHALL NOT contain a redeemable code, and no code path
SHALL create one — codes are inserted by an operator.

#### Scenario: The repository carries no live code

- **WHEN** the test suite runs
- **THEN** a test scanning the module finds no string literal that `promo_codes` would
  accept as a code, and fails naming the file if it does

#### Scenario: A code the table does not hold is refused

- **WHEN** an account submits a code with no row in `promo_codes`
- **THEN** the redemption is refused, and the refusal does not distinguish "no such code"
  from "not eligible for this code" in what it returns to the caller

### Requirement: A code is refused unless every bound admits it

A code SHALL be accepted only when it is active, unexpired, below its seat limit, and not
already redeemed by this account. Each bound is checked against the stored row at the moment
of redemption.

#### Scenario: The code is inactive

- **WHEN** an account redeems a code whose `active` is false
- **THEN** the redemption is refused and `uses` does not change

#### Scenario: The code has expired

- **WHEN** an account redeems a code whose `expires_at` is in the past
- **THEN** the redemption is refused and `uses` does not change

#### Scenario: The seats are gone

- **WHEN** an account redeems a code whose `uses` has reached its `max_uses`
- **THEN** the redemption is refused and `uses` does not change

#### Scenario: A null seat limit does not bound

- **WHEN** an account redeems a code whose `max_uses` is NULL and whose other bounds admit it
- **THEN** the redemption succeeds regardless of how large `uses` already is

#### Scenario: The same account redeems twice

- **WHEN** an account that already holds a `promo_redemptions` row for a code redeems it again
- **THEN** the redemption is refused

### Requirement: Seats are consumed atomically

The seat count SHALL be claimed in the same statement that tests it, so that concurrent
redemptions of the last seat cannot both succeed.

#### Scenario: Two accounts race for the last seat

- **WHEN** two redemptions of a code with one remaining seat are attempted concurrently
- **THEN** exactly one succeeds, the other is refused, and `uses` equals `max_uses`

### Requirement: An account redeems at most one code in its lifetime

An account SHALL hold at most one row in `promo_redemptions`. A second code, of any kind, is
refused. Stacking discounts is how a subscription becomes free by accident.

#### Scenario: A second, different code

- **WHEN** an account that has already redeemed a code redeems a different valid code
- **THEN** the redemption is refused, and the second code's `uses` does not change

### Requirement: Preview is authenticated, rate limited and side-effect free

The endpoint that reports whether a code is valid SHALL require an authenticated account,
SHALL be rate limited per account and per client address, and SHALL NOT consume a seat,
write a redemption, or otherwise change stored state.

#### Scenario: An anonymous caller previews a code

- **WHEN** a request without a session or key asks to preview a code
- **THEN** the request is refused with 401 and nothing is read from `promo_codes`

#### Scenario: A caller guesses codes in bulk

- **WHEN** an authenticated account exceeds the preview rate limit within the window
- **THEN** further previews are refused with 429 until the window passes

#### Scenario: Preview does not consume a seat

- **WHEN** an account previews a valid code with one seat left, then previews it again
- **THEN** both previews report it valid and `uses` is unchanged

### Requirement: A redeemed code discounts the first invoice only

A redeemed percentage SHALL apply to the first invoice of the subscription being bought and
SHALL NOT recur. Renewals are charged at list price.

#### Scenario: The subscription renews

- **WHEN** a subscription bought with a 90% code renews the following month
- **THEN** the renewal invoice carries no discount from that code
