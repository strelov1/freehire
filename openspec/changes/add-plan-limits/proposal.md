## Why

The credit ledger counts the wrong things. Measured on prod for 1–23 August 2026, the
assistant is **99.4% of model spend** ($248.19 of $250.63 at list price) and 81% of its
turns are the tailor workspace — yet the assistant, the chat and dictation debit nothing
at all. What the ledger does charge is priced two orders of magnitude below what it costs:
a match analysis debits 1 point against $0.0014 of model calls, while one assistant turn
runs 7.1 model calls and costs $0.48.

The gap is not an oversight in the rates, it is the position of the meter. Tailoring
debits three points **once, when the session is created**, and every turn inside that
session is then free. The heaviest account on the platform (`users.id = 601`) opened two
sessions, ran 54 turns, spent 6 points and consumed $25.18. That account is not abusing
anything — it is the ideal customer, doing exactly what the product is for, and the
system has no way to ask it for money.

Nothing here can be fixed by repricing points, because a balance of points is also the
wrong shape for what comes next: a Free and a Pro plan, where Pro is the same product
without the ceilings rather than a different product with more features.

## What Changes

- **BREAKING — the points currency is withdrawn.** A user no longer holds a balance.
  `GET /api/v1/me/credits` and `GET /api/v1/me/credits/history` are replaced by a plan
  and allowance surface; the SPA stops saying "credits" anywhere.
- **Metered actions become per-day allowances, per feature.** The reset moves from the
  calendar month to the calendar day (UTC). Unused allowance lapses; it does not bank.
- **Two plans.** Free gets every feature at a trial-sized daily allowance. Pro gets every
  feature with no user-facing ceiling, behind a fair-use guard set far above live
  behaviour so that a script cannot drain the gateway on one subscription.
- **The assistant, the chat and dictation start consuming an allowance.** They are the
  spend, and until now they were the only things not counted. This requires widening the
  `credit_ledger` feature CHECK, which today admits only `match` and `tailor`.
- **Tailoring is metered in two places, not one.** A daily count of sessions bounds
  breadth; a per-session turn ceiling bounds depth. Either alone leaves the current hole
  open — the 54-turn account passed the session count without noticing it.
- **BREAKING — `job-fit-analysis`'s 10-per-rolling-30-days quota is removed from the
  spec.** It was already superseded in code by the credit debit; the spec still describes
  it. The daily allowance replaces both.
- **BREAKING — the contribution reward is withdrawn**, because there is no longer a
  currency to award. Rewarding contributions returns in `add-invites` as days of Pro.
- **`users.pro_until` is introduced here** and is the only thing the hot path reads to
  decide a plan. It is populated by hand until `add-pro-subscription` connects
  RevenueCat; this change never calls an external billing API.
- A refusal stays **HTTP 402**, with a body naming the exhausted feature, the reset
  instant, and where to upgrade.

## Capabilities

### New Capabilities

- `plan-limits`: the plan a user is on, the per-day allowance each metered feature draws
  from, the per-session turn ceiling, how an allowance is reserved and released, and the
  402 a refusal renders.

### Modified Capabilities

- `job-fit-analysis`: the rolling-30-day quota and the credit debit are both replaced by
  a per-day allowance; the 402 body changes shape.
- `cv-tailoring`: the debit moves from "once per session" to a session that both consumes
  a daily allowance and carries a turn ceiling.
- `assistant-sessions`: an assistant turn consumes an allowance and can be refused; it
  was previously unmetered.
- `assistant-voice-dictation`: a transcription consumes an allowance in addition to its
  existing per-caller rate limit.
- `api-keys`: a `cv`-scoped key is refused on allowance-consuming endpoints — the rule is
  unchanged, the thing it names is no longer "credits".
- `link-contributions`: recording a board no longer awards a currency.
- `tailor-preflight-check`: the check must not consume a tailoring allowance (previously
  "credit"); the guarantee is the same, its unit is not.
- `tailor-job-match`: the deterministic score consumes no allowance.
- `account-deletion`: what is erased is the ledger and the plan record, not a balance.
- `llm-spend-attribution`: usage is reported over the day the allowance resets on, not
  the month the balance reset on; the credits page it points at is replaced.

## Impact

- **Schema:** widen the `credit_ledger` feature CHECK; add `users.pro_until`; the grant
  and debit indexes move from a month key to a day key. `credit_balances` loses its
  reason to exist as a balance and becomes a per-day, per-feature consumption row.
- **Go:** `internal/ai/credits` is reshaped into the plan/allowance service — it keeps
  its idempotency-by-`(user, feature, ref)`, its `SELECT FOR UPDATE`, its lazy period
  reset and its `Release`. Call sites gain the assistant runner, the dictation handler
  and the assistant chat alongside `fitanalysis` and `cv_tailor`.
- **HTTP:** `/me/credits*` replaced; every metered route can now answer 402.
- **SPA:** the credits page and `CreditsView.svelte` are re-cast as today's usage; the
  balance widgets and the word "credits" are removed.
- **Ops:** `CREDITS_*` environment variables are replaced by plan configuration. Prod
  carries live `credit_ledger` rows, so the migration is a reshape of a populated table,
  not a fresh install.
- **Not in this change:** RevenueCat and payment, invites and gifted Pro days, and
  reducing the $0.48 turn (prompt caching, model choice, step ceiling) — the last is a
  known debt that decides whether $5/month is profitable, and is measurable today through
  Bifrost's `x-bf-dim-feature` header.
