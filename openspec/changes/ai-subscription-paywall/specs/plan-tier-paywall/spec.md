## Purpose

Makes every currently metered AI feature subscription-only by default: the free tier's
daily allowance is zero and refusal is enforced rather than merely counted, while
preserving the lever to reopen a free allowance for one feature without a deploy, and
keeps the account plan page's upgrade calls to action honest about which tier is actually
for sale.

## ADDED Requirements

### Requirement: Free tier has no daily allowance for metered AI features

The system SHALL configure the free tier's daily allowance to zero, by default, for every
currently metered AI feature: CV tailoring, job-fit analysis, the assistant, dictation and
cover-letter drafting.

#### Scenario: A free account is refused a metered AI feature

- **WHEN** a signed-in free-tier user requests a metered AI feature (a new tailoring
  session, a fit analysis, an assistant turn, a dictation transcription, or a cover letter
  draft)
- **THEN** the request is refused with HTTP 402 before any model call is made, and the
  response names the exhausted feature and when its allowance resets

#### Scenario: A subscribed account is unaffected

- **WHEN** a Pro or Ultra subscriber requests the same metered AI feature
- **THEN** the request proceeds normally, subject only to the paid tier's fair-use guard

### Requirement: Refusal is enforced, not shadowed, by default

The system SHALL ship every currently metered AI feature with refusal enforcement on by
default, rather than the shadow mode where usage is only counted and nobody is refused.

#### Scenario: Default configuration enforces every feature

- **WHEN** the plan configuration is read with no environment overrides set
- **THEN** every metered AI feature reports itself as enforcing, and a free account past
  its (zero) allowance is refused rather than recorded as a would-have-been refusal

### Requirement: A free allowance can be reopened for one feature without a deploy

The system SHALL continue to let a positive `PLAN_FREE_DAILY_<FEATURE>` environment
override raise one feature's free daily allowance above zero, without a code change.

#### Scenario: Operator raises one feature's free allowance

- **WHEN** `PLAN_FREE_DAILY_<FEATURE>` is set to a positive number for one metered feature
- **THEN** that feature's free daily allowance becomes that number, and every other
  feature's free daily allowance is unaffected

### Requirement: An upgrade call to action only advertises a tier that is for sale

The account plan page's upgrade call to action for a Pro subscriber SHALL name and link to
Ultra only while at least one Ultra price is currently offered for sale; when no Ultra
price is offered, no Ultra-specific call to action is shown to that subscriber.

#### Scenario: Ultra is currently offered

- **WHEN** a Pro subscriber views their account plan page and at least one Ultra price is
  currently offered
- **THEN** the page shows an "Upgrade to Ultra" call to action linking to the pricing page

#### Scenario: Ultra has been pulled from sale

- **WHEN** a Pro subscriber views their account plan page and no Ultra price is currently
  offered
- **THEN** the page shows no Ultra-specific upgrade call to action, though the
  subscriber's own subscription-management link, if any, is unaffected
