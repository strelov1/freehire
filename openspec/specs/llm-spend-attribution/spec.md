# llm-spend-attribution Specification

## Purpose

Naming the account behind every LLM call at the gateway, so what the product spends can be
attributed to a person and to a feature.

Work done for a signed-in user goes out on that user's own gateway credential, minted
lazily and never shown to them; work that belongs to nobody — catalogue enrichment,
Telegram extraction, embeddings — keeps the service credential. Attribution fails open
everywhere: it can delay nothing, refuse nothing, and fail no call.

This capability MEASURES. It does not bound: the gateway supports a per-account ceiling and
the configuration passes one through, but none is set, because a limit chosen before the
spend distribution is known is a guess. Pricing the product remains the credits ledger's
job; a gateway budget is a fuse, and the two are not the same instrument.

## Requirements

### Requirement: Work done for a user is spent under that user's name

Every LLM call made on behalf of a signed-in user SHALL travel on a gateway credential
that identifies that account, and no other. The credential is minted on the account's
first AI call and reused thereafter.

This is what makes the gateway's own per-account spend records true. Until it holds, every
call is anonymous and no cost question about a person can be answered at all.

#### Scenario: The first call mints the credential

- **WHEN** an account with no credential yet makes its first AI call
- **THEN** a credential naming that account is minted, stored, and used for the call

#### Scenario: Later calls reuse it

- **WHEN** the same account makes a second call
- **THEN** the stored credential is reused and nothing new is minted

#### Scenario: Two accounts are told apart

- **WHEN** two accounts have each run AI work
- **THEN** the gateway attributes each account's spend separately

### Requirement: Work that belongs to nobody keeps the service credential

Calls that are not made on behalf of a user — catalogue enrichment, Telegram extraction,
embedding, mail classification — SHALL use the service credential and MUST NOT borrow any
user's.

A catalogue vacancy has no owner. Attributing its enrichment to whichever account happened
to trigger a run would put a cost on a person who did not incur it, which is worse than
leaving it unattributed.

#### Scenario: A worker run

- **WHEN** an enrichment worker processes the queue
- **THEN** its calls go out on the service credential and no user credential is resolved

### Requirement: The credential is invisible to the user

The credential SHALL never be shown to the account it belongs to, never be required from
them, and never appear in any API response. No page, field, or setting exposes it.

It is infrastructure bookkeeping, not a feature. A person signing in to look for work has
no use for a gateway key and no way to judge one.

#### Scenario: A user who never heard of it

- **WHEN** a signed-in user runs an assistant turn having never configured anything
- **THEN** the turn behaves exactly as before and the user is shown nothing about credentials

#### Scenario: The credential does not leak into a response

- **WHEN** any endpoint that touches AI returns its response
- **THEN** the credential value appears in no field of it

### Requirement: Attribution never costs a call

A failure to resolve, mint, or use a user's credential SHALL NOT fail the call. The call
proceeds on the service credential and the failure is logged.

The fuse fails open on purpose. Losing the ability to say who spent something is our
bookkeeping problem; refusing a person their answer over it would be trading a real service
for a record of it.

#### Scenario: The gateway's admin API is unreachable

- **WHEN** minting a credential fails
- **THEN** the call proceeds on the service credential, succeeds, and the failure is logged

#### Scenario: The gateway no longer recognises a stored credential

- **WHEN** a stored credential is rejected as unknown
- **THEN** a replacement is minted and stored, and the call completes

### Requirement: A credential stops spending with its account, but is not erased

Deleting an account SHALL stop its gateway credential from being able to spend. It MUST
NOT erase that credential, because the gateway's record of what was spent with it hangs
off the key itself. A gateway that cannot be reached at that moment MUST NOT stop the
account from being deleted.

A departing member must lose the ability to spend; they do not have to take the cost
history with them. What remains on the gateway is a blocked credential labelled with an
internal numeric id, which maps to nobody once the account row is gone — the deletion is
what anonymises it.

Availability follows the rule the OAuth grant already follows: a member leaving must not be
held up by a third system.

#### Scenario: An account is deleted

- **WHEN** a member deletes their account
- **THEN** their gateway credential can no longer spend, and the record of what it spent remains

#### Scenario: The gateway is unavailable during deletion

- **WHEN** retiring the credential fails
- **THEN** the account deletion still completes and the failure is logged

#### Scenario: A credential that was never spent with

- **WHEN** a credential is abandoned before it is ever stored — the loser of a minting race
- **THEN** it is erased outright, having no history worth keeping

### Requirement: Each call names the feature it served

Every call SHALL carry a tag naming the feature it was made for, distinguishing at least the
assistant turn, its follow-ups, match analysis, CV extraction, the ATS review and the
autofill planner. An assistant turn SHALL additionally carry its preset, because a
rehearsal, an unattended tailoring run and a question cost wildly different amounts and are
otherwise one number.

CV tailoring has no tag of its own, and must not gain one: `/me/cvs/tailor` makes no model
call — it mints a CV and debits credits, and the work is an assistant turn under the
`tailor` preset. A second tag would count one spend twice.

The question this capability exists to answer is which feature is expensive. Without the tag
the gateway can only report a model, and every feature runs on the same one.

#### Scenario: Totals per feature

- **WHEN** more than one feature has run over a period
- **THEN** their spend can be summed per feature for that period

#### Scenario: Totals per assistant preset

- **WHEN** turns have run under more than one preset
- **THEN** their spend can be summed per preset for that period

### Requirement: A caller can see what their own account did

`GET /api/v1/me/usage` SHALL report the authenticated caller's own AI activity for the
current period — model calls, failures and tokens. It MUST report only theirs, MUST answer
a caller with no activity as zeroes rather than as an error or a 404, and MUST NOT fail the
request when the gateway is unreachable.

It SHALL NOT report cost in any currency. The gateway prices from a list against a mixed
upstream pool, so the figure is neither what the operator pays nor what the caller pays —
their price is credits, reported over this same calendar month. Two numbers in two
currencies for one thing would leave the fictional one indistinguishable from the real one.

The period SHALL be the calendar month the credits balance already resets on, so a balance
and an activity count are never reported against different months.

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

#### Scenario: The reader is shown it only in beta

- **WHEN** an account that is not a beta tester opens the credits page
- **THEN** the activity panel is not rendered, and the balance and history are unchanged

### Requirement: No ceiling is imposed unless one is configured

A per-user spend ceiling and request-rate limit SHALL be configuration, absent by default.
With neither configured, no call is ever refused for exceeding a limit.

A ceiling chosen before the spend distribution is known is a guess, and a guess that refuses
people is expensive to be wrong about. This change exists to produce that distribution; the
number it informs is chosen afterwards.

#### Scenario: An unconfigured deployment

- **WHEN** no ceiling is configured
- **THEN** credentials are minted without one and no call is ever refused for spend

#### Scenario: A configured ceiling is reached

- **WHEN** a ceiling is configured and an account exceeds it
- **THEN** the gateway's refusal surfaces as an ordinary model failure, through the paths that
  already handle one
