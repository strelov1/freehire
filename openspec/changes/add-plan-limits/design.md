## Context

`internal/ai/credits` is a working, careful piece of machinery pointed at the wrong
target. It holds an append-only `credit_ledger`, a materialised `credit_balances` cache, a
`SELECT FOR UPDATE` debit that serialises concurrent first-ever spends, idempotency by
`(user, feature, ref)` and by `(user, period)` for grants, a lazy monthly reset that needs
no cron, banking semantics that keep rewards above the grant, and — since d5df7abd — a
`Release` that gives a reservation back when the work produced nothing. None of that is
wrong. What is wrong is what it counts and when it counts it.

Measured on prod, 1–23 August 2026: the assistant is 99.4% of model spend and debits
nothing; tailoring debits three points at session creation and nothing for the 54 turns
that follow; a fit analysis debits one point for $0.0014 of model calls. The ledger is
precise about the cheapest thing and silent about the expensive one.

The product is also changing shape. Free and Pro are being introduced, where Pro is the
same product without the ceilings — so what a user needs to understand is "2 CV edits
today", not "5 points, and tailoring costs 3". A balance of an invented currency is the
wrong unit to sell, and a monthly reset is the wrong rhythm: a month's worth spent on the
3rd leaves 27 days with no reason to return.

Constraints that shape everything below: the meter sits on the request path of every AI
feature, so it must not call anything over the network; prod carries live `credit_ledger`
rows and live users mid-flow; `internal/platform/arch/layering` enforces the block graph,
so where this package lives is a real decision, not a filing preference.

## Goals / Non-Goals

**Goals:**

- One question, one answer: "may this user do this action now?" — resolved from stored
  state, with no network call and no knowledge of billing.
- Per-feature, per-UTC-day allowances that reset lazily, with no scheduled job.
- Two bounds on tailoring — sessions per day and turns per session — because the
  production hole is depth, not breadth.
- Metering that finally covers the assistant and dictation, which are the spend.
- Keep the correctness the credit ledger already earned: atomic reservation, idempotency
  by reference, release on failure, append-only history.
- Ship the counting before the refusing, so the numbers can be checked against real
  behaviour before anyone is told "no".

**Non-Goals:**

- RevenueCat, checkout, webhooks, and the reconciler — `add-pro-subscription`.
- Invite codes and gifted Pro days — `add-invites`.
- Reducing the $0.48 turn. It is the reason $5/month does not yet pay for itself, it is
  measurable today through Bifrost's `x-bf-dim-feature`, and it is a separate change.
- One-off packs, annual plans, team accounts, proration, refunds.

## Decisions

### A new `usage_ledger`, beside the old `credit_ledger`, not a reshape of it

The obvious move is to widen `credit_ledger`'s CHECK, swap `period` from `'YYYY-MM'` to
`'YYYY-MM-DD'` and re-point the indexes. It is also the move that leaves one column
holding two incompatible meanings: every historical row would carry a month in a field the
new code reads as a day, and `applicableRemaining`'s banking rule would apply to rows that
no longer describe a balance.

Instead: a new `usage_ledger` (user, feature, day, ref, delta, kind) plus a `usage_daily`
materialised counter keyed `(user, feature, day)`. `credit_ledger` and `credit_balances`
are left exactly as they are, written by nobody, read by nobody except account deletion.
They are dropped in a later change, once the new path has run long enough that nobody
wants the old numbers back.

**Alternative considered:** migrate the historical rows by taking each month's row as day
one of that month. Rejected — it fabricates a fact (that the consumption happened on the
1st) into an append-only ledger, which is exactly the kind of lie that is impossible to
detect a year later.

**Cost accepted:** two tables that look alike for one release, and account deletion has to
erase both. Both are cheap and visible; a column with two meanings is neither.

### The package stays in the `ai` block, renamed `internal/ai/plan`

The plan is a property of a user, which argues for `identity`. But `identity` and `ai` sit
on the same layer, so a package in `identity` could not be called from `internal/ai/assistant`
or `internal/ai/speech` — which is where two of the five metered features live. The rule
would be satisfied and the code would be impossible.

So the package stays in `ai`, where `credits` already is, and where `candidate/fitanalysis`
(layer 4) and the handlers (layer 8) already reach it. It reads `users.pro_until` through
`db.Queries` (platform, layer 1) rather than through an `identity` service — a single
column read, not a use of identity's behaviour. The layering table in
`internal/platform/arch/layering/blocks.go` gains the renamed package.

**Alternative considered:** put the plan in `identity` and the metering in `ai`, with the
plan passed in from the caller. Rejected — every call site would then have to know how to
resolve a plan before asking whether an action is allowed, which is precisely the knowledge
this package exists to hold. The seam would leak into eleven places to satisfy a filing rule.

### The plan is a timestamp on `users`, and the hot path reads nothing else

`users.pro_until timestamptz NULL`. Future means pro; NULL or past means free. No plans
table, no entitlements cache, no provider call.

This is what lets `add-pro-subscription` be a genuinely separate change: RevenueCat's
webhook and reconciler write this column and nothing else, and if RevenueCat is down for
an hour, every metered action still resolves instantly and correctly from the last thing
we were told.

**Alternative considered:** a `subscriptions` table with status, provider, product and
period. Rejected as premature — everything the hot path needs is the timestamp, and the
provider's own record is the source of truth for the rest. The table can arrive with the
change that has something to put in it.

### Two bounds on tailoring, and the second one is a ledger reference, not a column

The daily session count is an ordinary allowance. The per-session turn ceiling is stored
as ledger entries whose `ref` is `<session_id>#<n>` — the first entry is `#1`, and buying
another ceiling's worth writes `#2`. The ceiling in force is `15 × (count of entries for
that session)`, and the turns used come from `assistant_messages`, which already records
them.

This falls out of the idempotency key rather than fighting it: `(user, feature, ref)` is
unique, so a plain `session_id` ref could only ever be charged once — which is the correct
behaviour for starting a session and the wrong behaviour for extending one. The `#n` suffix
makes an extension a distinct, still-idempotent event, and a double-clicked "continue"
consumes one allowance rather than two.

**Alternative considered:** a `turn_allowance` integer on `assistant_sessions`, incremented
on purchase. Rejected — it is a second place where consumption is recorded, and it can
disagree with the ledger. Reading it from the ledger keeps one source of truth for "what
was paid for".

### Count first, refuse later: enforcement is a switch, and it starts off

The allowance numbers come from three weeks of one August. They are the best evidence
available and they are still a guess about behaviour under a paywall that does not exist
yet. Turning them on and being wrong means refusing real users at a number nobody checked.

So the metering ships in two states. First, it writes the ledger and reports usage while
never refusing — a shadow run that answers "how many people would have hit this, and at
which feature?" against live traffic. Then, once the numbers are read, the same code
refuses. The switch is configuration, per feature, so a single badly-chosen ceiling can be
released without a deploy.

**Alternative considered:** ship enforcing and adjust from complaints. Rejected — the
first thing a paywall does is teach people the product says no; being wrong on day one is
paid for in accounts that never come back, and the shadow run costs one release.

### Plan configuration is a table in code, not scattered constants

One structure names every plan, every feature, its daily allowance, its fair-use guard and
whether enforcement is on. Call sites name a feature and nothing else. This is what makes
"the free plan gives 3 analyses" a single edit, and it is what stops the ceilings from
drifting apart across five handlers the way the four legal-form vocabularies did.

`CREDITS_MONTHLY_GRANT`, `CREDITS_COST_MATCH`, `CREDITS_COST_TAILOR` and
`CREDITS_CONTRIBUTION_REWARD` are removed. Their replacements are per-plan, per-feature and
are read once at construction.

### A refusal is 402, and on a stream it precedes the stream

402 is already the shape the SPA handles, and `renderCreditsRefusal` already exists — it is
re-pointed, not rewritten. The one rule worth restating: the streaming fit endpoint and the
assistant SSE must issue the status BEFORE the stream opens. An error frame inside a
200 stream is invisible to anything that checks status codes, and the SPA would render an
empty answer instead of an upgrade prompt.

## Risks / Trade-offs

**The numbers are drawn from 101 users over 23 days, under no paywall at all.** → The
shadow run is the mitigation: enforcement is off until the ledger has been read against
live traffic, and each ceiling is independently switchable.

**Two ledgers exist for a release, and account deletion must erase both.** → The deletion
spec is amended in this change, and a test asserts no row keyed to a deleted user survives
in either table. The old pair is dropped in a follow-up once the new path is trusted.

**Metering the assistant means a turn can now fail for a new reason, on a path where
failure is already awkward (streams, resumes, disconnects).** → Reservation happens before
the first model call, release on every terminal failure, and idempotency by turn reference
so a resume is not charged twice. The existing `assistant-turn-survives-disconnect`
behaviour is the thing most likely to expose a mistake here, and it gets an explicit test.

**A daily reset invites a user to wait until midnight UTC rather than upgrade.** → Accepted
deliberately. The alternative — a rolling window — is harder to explain, and the point of
a daily allowance is that the user comes back tomorrow. Coming back is the behaviour we
want; the upgrade is sold to the person who does not want to wait.

**Free at 2 tailoring sessions a day is generous against measured behaviour (2 sessions per
user per three weeks).** → Also deliberate. A free tier that pinches on the first visit
sells nothing because nobody has seen the product work. The bound that protects spend is
the 15-turn ceiling, which is where the heavy account actually lived.

**`pro_until` is populated by hand until the next change lands.** → That is the intended
seam, and it is also how the shadow run gets a pro-plan test subject. The risk is forgetting
to remove a hand-set value; it expires by itself, which is the property that makes a
timestamp the right shape here.

## Migration Plan

1. Migration adds `users.pro_until`, `usage_ledger`, `usage_daily`. Additive only — no
   existing column or table is altered, so the old code keeps running against the same
   database and rollback is a deploy, not a restore.
2. Ship with enforcement off for every feature. The ledger fills; nothing is refused.
3. Read the shadow ledger after a week: per feature, how many users would have been
   refused, and at which point in their day. Adjust the numbers in configuration.
4. Turn enforcement on feature by feature, cheapest first (fit analysis), tailoring last —
   it is the one with two bounds and the most user-visible consequence.
5. SPA switches from balance language to usage language in the same release as step 4 for
   the feature concerned, so the interface never describes a limit that is not enforced.
6. `credit_ledger` and `credit_balances` are dropped in a later change, not this one.

**Rollback:** enforcement off is a configuration change, not a deploy. Beyond that, the
new tables are additive and unread by the old binary, so reverting the deploy restores the
previous behaviour with no data loss — the historical balances were never touched.

### Why the gateway's own budget does not replace this

Bifrost already meters: a virtual key carries `budgets: [{max_limit, reset_duration}]` and
a request-rate limit, and `internal/ai/llmkey` already mints them. The obvious question is
whether that makes this package redundant. It does not, and the reason is what each thing
bounds.

A gateway budget bounds MONEY. It answers "will we go broke", and it answers it better than
counting actions ever could — the same two CV edits cost $0.50 or $15 depending on how deep
they go. What it cannot express is what a candidate was PROMISED. "2 CV edits a day" is
something a person can plan around; "$0.20 a day" on a free tier is a bank statement, not an
offer. The gap between them is the 7.1 model calls one turn makes: the person presses once,
and how many calls follow is the model's decision, not theirs.

Five things a per-key budget cannot do at all, each of which this change relies on: split by
feature (one key, one budget — splitting it needs four keys per user, which dismantles the
per-user spend attribution the key exists for); bound the turns inside one tailoring session
(the gateway has no idea sessions exist); leave a retry uncharged (a resumed turn is fresh
calls, and they cost); give an allowance back when the work produced nothing (tokens are
spent); and refuse BEFORE the stream opens — a gateway refusal arrives mid-call, inside a
response that already returned 200.

Shadow mode is the sixth: "count but do not refuse" is not expressible as a budget, and it
is what lets this ship without breaking anybody.

Two facts sharpen it. `LLMUserMaxBudget` is already implemented and deliberately unset, for
this change's own reason — a ceiling chosen before the spend distribution is known is a
guess (`internal/platform/config/config.go:115-119`). And dictation runs on the SERVICE
credential (`cmd/server/main.go:223`), so a per-user budget does not see it at all.

**Where the gateway IS the better answer: the fair-use guard.** It is infrastructure defence
rather than product, so money is the honest unit and a mid-stream refusal is acceptable —
what reaches it is automation. It stays here for now only because the pool runs on free and
scavenged keys, which leaves `MaxBudget` with nothing to calibrate against; moving it is a
change of its own, once there are real bills to read.

## Open Questions

- **What does the fair-use guard cost when it fires?** The spec says operators are told and
  the user is not shown a plan limit. Whether that is a log line, a Prometheus counter, or a
  Sentry event is left to implementation, but it must not be silent — a guard nobody sees
  fire is a guard that will be blamed for an outage it did not cause.
- **Does the extension surface need its own allowance display?** The tailor preflight prompt
  is shown from the extension side panel as well as the web app, and it now states what a
  session costs. Whether the extension can read the plan without a new endpoint is not yet
  established.
