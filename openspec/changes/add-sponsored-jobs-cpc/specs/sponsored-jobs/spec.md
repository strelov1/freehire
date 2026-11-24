## Purpose

Lets anyone pay freehire per valid click to show an open vacancy in labelled sponsored slots,
with a prepaid budget that can never be overspent.

## ADDED Requirements

### Requirement: A signed-in user creates a campaign for an open job

The system SHALL let any authenticated user create a sponsored campaign for any open job. The
user sets a CPC in USD cents at or above the configured floor, and optionally a daily cap. The
system SHALL refuse a campaign for a closed job and a CPC below the floor.

#### Scenario: Campaign for an open job is created as a draft

- **WHEN** a signed-in user creates a campaign with a CPC at or above the floor for an open job
- **THEN** a campaign is stored in status `draft`, owned by that user

#### Scenario: Campaign for a closed job is refused

- **WHEN** a user creates a campaign for a job whose `closed_at` is set
- **THEN** the request is refused and no campaign is stored

### Requirement: Budget is prepaid through Stripe Checkout

The system SHALL fund a campaign only through a one-off Stripe Checkout payment of at least the
configured minimum top-up. It SHALL credit the campaign budget exactly once per checkout
session, when the signed webhook reports the session completed.

#### Scenario: Completed checkout credits the budget once

- **WHEN** the Stripe webhook delivers `checkout.session.completed` for a campaign top-up twice
- **THEN** the campaign budget grows by the paid amount exactly once

#### Scenario: Top-up below the minimum is refused

- **WHEN** a user starts a top-up below the configured minimum
- **THEN** no checkout session is created

### Requirement: A paid campaign runs only after moderator approval

The system SHALL put a first-funded campaign into `pending_review`, and SHALL serve it only
after a moderator approves it. A rejected campaign SHALL be refunded in full and never serve.

#### Scenario: Approval activates the campaign

- **WHEN** a moderator approves a `pending_review` campaign
- **THEN** it becomes `active` and its job is re-indexed as sponsored

#### Scenario: Rejection refunds the payment

- **WHEN** a moderator rejects a `pending_review` campaign
- **THEN** it becomes `rejected`, a full Stripe refund is issued for its top-ups, and it never
  appears in any placement

### Requirement: Sponsored jobs appear in search, alert emails and the company page

The system SHALL show active sponsored jobs, labelled "Sponsored", in three places:
- up to 2 per first page of a search, matching the same query and filters, highest CPC first;
- at most 1 per saved-search alert email, matching that subscription's query;
- pinned on top of their company's page.

The system SHALL NOT show sponsored jobs on later search pages or on the unfiltered jobs feed.
A failure to fetch sponsored jobs SHALL NOT fail the organic search.

#### Scenario: First search page carries matching sponsored jobs

- **WHEN** a visitor searches for a query and two active campaigns' jobs match it
- **THEN** the first page contains those two jobs labelled sponsored, and neither appears again
  among the organic results of that page

#### Scenario: Non-matching sponsored job is not shown

- **WHEN** an active campaign's job does not match the query or filters
- **THEN** it does not appear in that search

#### Scenario: Sponsored lookup failure leaves organic results intact

- **WHEN** the sponsored lookup errors or times out
- **THEN** the organic results are returned normally without sponsored entries

### Requirement: Clicks are validated and charged server-side without overspend

The system SHALL route every sponsored placement through a signed redirect. It SHALL always
redirect to the job's URL, and SHALL record every click with a validity reason. It SHALL charge
only valid clicks, at `min(CPC, remaining budget, remaining daily cap)`. A click is invalid when:
- it comes from a known bot;
- the same visitor already clicked that campaign within 24 hours;
- the rate limit is exceeded;
- the campaign is not active or has no budget left.

The total charged SHALL never exceed the funded budget.

#### Scenario: Valid click is charged

- **WHEN** a non-bot visitor clicks a sponsored link of an active campaign for the first time in
  24 hours
- **THEN** the visitor is redirected to the job URL and the campaign is charged its CPC

#### Scenario: Repeat click is free

- **WHEN** the same visitor clicks the same campaign again within 24 hours
- **THEN** the visitor is redirected, the click is recorded as `duplicate` and nothing is charged

#### Scenario: Last cents exhaust the campaign

- **WHEN** a valid click arrives and the remaining budget is less than the CPC
- **THEN** only the remaining budget is charged, the campaign becomes `exhausted`, and its job
  leaves all placements

#### Scenario: Concurrent clicks cannot overspend

- **WHEN** many valid clicks for one campaign arrive at once with budget for only one
- **THEN** exactly one is charged and total spend equals the budget

### Requirement: The buyer sees delivery and spend

The system SHALL show the campaign owner, per campaign and day: impressions served, valid
clicks, invalid clicks by reason, spend, and remaining budget.

#### Scenario: Dashboard reflects charged clicks

- **WHEN** a campaign received 3 valid and 2 duplicate clicks today
- **THEN** its dashboard row for today shows 3 valid clicks, 2 invalid (`duplicate`), and spend
  equal to the sum charged
