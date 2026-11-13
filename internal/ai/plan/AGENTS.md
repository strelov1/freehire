# Plan and allowance conventions

## Scope
The plan a user is on and the per-day allowance each metered feature draws from. Five of the
six are AI features whose cost is a model call; the sixth, auto-apply, is a headless browser
submitting an application, and it is shaped differently for that reason wherever it differs.
One
question in, yes or no out: `Consume(user, feature, ref)`. Which plan, what it allows, how
much is left and whether a refusal is switched on are all this package's business.

The rule itself is a pure function (`decide`, `decideTurn`) of the plan, the feature, what
the day already holds and whether the reference was charged before — so every branch is
tested without a database, including the ones that are awkward to reach through a
transaction. `store.go` is the transaction around it and nothing else.

## Always true

- **Three tiers, and the better one still live wins.** `TierOf` checks ultra first and it
  wins even when a pro entitlement reaches FURTHER: both can be live at once — the provider's
  portal makes that ordinary during an upgrade — and a resolution that could go DOWN when
  somebody spends more is a bug people notice in the worst possible way.

- **A rule meaning "somebody is paying" is `Tier.Paid()`, never `== TierPro`.** That shape
  is what excluded Ultra from the tailoring turn ceiling the first time it was written, so
  buying the more expensive plan would have cut a session short where the cheaper one ran
  on. Written as "paid", a fourth tier gets the rule for free.

- **One allowance per tier, and where a tier is unlimited its number is the FAIR-USE guard
  rather than a ceiling.** That convention is why `decide` names no tier at all: it reads
  `Unlimited` off the allowance and applies the guard to whichever tier has one. The shape
  replaced a free number plus a pro fair-use figure, which could only say "free has a
  ceiling, pro does not" — and auto-apply needs the third shape, a real daily ceiling on pro.

- **Free gets no AI at all, and that is a rule about the TIER, not a number.** `decide`
  refuses a tier that is not `Paid()` before it reads any allowance — before even the
  already-charged branch, because a recompute is still a model call and a lapsed
  subscription does not keep buying them. There is no free allowance to configure:
  `featureConfig` has no free field, `PLAN_FREE_DAILY_<FEATURE>` is no longer read (it once
  reopened one without a deploy, and a lever that can only weaken an invariant is one an
  operator pulls by accident), and `WithFreeDaily` is gone. `RequirePaid` is the same
  question asked on its own, for the AI actions that have no allowance (the ATS review, a
  re-run of extraction, mail recall, autofill): `ErrPaymentRequired` for free, the raw
  error for a failed read — which a caller refuses on (503) and never authorises, the
  opposite of the meters' fail-open rule below, because what it protects is model spend.
  `WithPaidAI`/`PaidAI` mark a context as having passed it; `internal/api/handler`'s
  binding refuses to hand out a model client without that mark — or without
  `WithFreeAI`, the named exception for the calls every account gets: the stored-CV parse,
  the onboarding CV read (`POST /onboarding/cv`, the only one open to a guest), the search
  interpretation ("Describe with AI") and the support agent. Support is
  `FeatureSupport`, deliberately NOT in `AllFeatures()` — it is not part of a plan, so
  no tier table, plans page or upgrade nudge touches it. `ConsumeSupport` draws on ONE flat
  daily figure for every account (`SUPPORT_DAILY_MESSAGES`, default 20); a conversation holds
  `SUPPORT_TURNS_PER_SESSION` messages (default 12) and the day's total across all accounts is
  bounded by `SUPPORT_DAILY_MESSAGES_GLOBAL` (default 2000, a soft pre-check). Its refusals
  never carry an upgrade link and never feed `recordLimitHit`. Tests that need an
  allowance to consume partway into pin a real PRO ceiling with `WithProCeiling`.

- **The paid gate is enforced twice in `internal/api/handler`, and a few callers ask
  themselves.** The `mw.paidAI(feature)` middleware (`paid_ai.go`) answers an unpaid caller
  402 — in the body every plan refusal carries, before any stream opens — and an unreadable
  plan 503, never a pass. The user-call binding (`llmBinding.bind/caller`, `user_llm.go`)
  refuses a model client for a context without `WithPaidAI`/`WithFreeAI`, so a new
  model-calling route mounted without the middleware fails its first test; a test also
  forbids `llmkey.Bind/Caller` anywhere in the handler package outside the binding. Callers
  the middleware cannot reach call `RequirePaid` themselves: the auto-apply orchestrator's
  tailor run (for the queue entry's owner) and `cmd/auto-apply`'s drafting (a lapsed
  candidate's attempt parks). Cached AI results, deterministic scoring, search and the
  public CV roast stay open to free; ownerless background work (enrichment, embeddings,
  Telegram) is unaffected.

- **The `WithFreeAI` exceptions are marks, not missing checks.** The CV upload's background
  extraction and its retry run for every account on their own cheap model
  (`RESUME_EXTRACT_MODEL`, falling back to `LLM_MODEL`). Every upload entry point
  (`PUT /me/resume`, `/me/resume/extract`, the MCP `add_cv` tool, which takes only a first CV)
  reaches it through one `storeCV`, and the mark is set inside the derivation, not by the
  caller. The onboarding CV read (`onboarding_cv.go`) runs the same extractor in the request,
  for a visitor with or without an account, and stores nothing: a guest spends the service
  credential (never the binding, which would mint a key for account 0), and the operations
  gateway's `onboarding-cv-guest` budget bounds guests per IP — the app holds no counter. The
  parse was paid until 2026-10-19
  and that cost paying customers: a CV uploaded before upgrading was never parsed, upgrading
  did not re-run it, and the experience bank every paid feature reads stayed empty. Support
  (`POST /support/sessions/:id/messages`, `assistant_support.go`) meters itself through
  `meterSupport`, which fails CLOSED — unlike the paid meter, because a counter that cannot
  be read must not become unbounded spend: the per-account daily figure answers 402, the
  per-session and global figures 429. `SUPPORT_MODEL` picks its model (falling back to
  `LLM_MODEL`); `ONBOARDING_REPLY_TO` is the inbox its handover mail goes to. The generic
  `/assistant/sessions/:id/...` routes answer a support session 404 — on the ones behind
  `mw.paidAI` (messages, opening, autopilot, …) only once that gate passes, so a free caller
  gets its 402 first.

- **A paid-gate refusal is recorded as a paywall hit, and the record never touches the 402.**
  `plan.decide` refuses the free tier before any allowance is read, so those refusals never
  reach `plan_limit_hits` — the two tables are different questions ("the plan never included
  it" against "a ceiling was reached"). `Store.RecordPaywallHit` writes `paywall_hits`
  (migration 0248), once per (user, feature, job, day) with `ON CONFLICT DO NOTHING`: one row
  per refusal would let a free account grow the table without bound by retrying or iterating
  job slugs, and "who hit which wall" needs only that it happened and when it first did. The
  job is resolved INSIDE the insert from a bound identifier — a job slug, or the caller's own
  CV or assistant session — so it costs no round trip, stores no request text, and a foreign
  or unknown identifier yields a NULL job, never another user's. The gate
  (`internal/api/handler/paid_ai.go`) calls it from a detached goroutine (`WithoutCancel`, a
  5 s timeout, its own `recover`), because an in-request insert would add a database round
  trip to every 402 and a failed write must not change it; a failure is logged and dropped.
  Not recorded: a 503 (an unreadable plan is not a paywall hit), a paid caller, a metered
  ceiling (that is `plan_limit_hits`), and the auto-apply orchestrator's tailor refusal (it
  acts for a queue owner, nobody pressed a button). `feature` is the plan feature name, or
  `ats_review` / `mail_recall` / `autofill` for the three routes with no allowance, chosen
  from the route template. The table links an account to jobs it looked at; it is erased with
  the account (`ON DELETE CASCADE`), and has no retention sweep yet.
  `cv-lifecycle-mail` reads it: about an hour after a hit, the account gets one letter naming
  the feature and the job (see [internal/engage/cvlifecycle/AGENTS.md](../../engage/cvlifecycle/AGENTS.md)).
  A new paid-AI route label needs its words in `cvlifecycle.attempts`.

  Operator report, who hit which paywall in the last 7 days (change `7` for another window):

  ```sql
  SELECT h.feature, h.user_id, j.public_slug AS job_slug, h.day, h.created_at
  FROM paywall_hits h
  LEFT JOIN jobs j ON j.id = h.job_id
  WHERE h.created_at >= now() - interval '7 days'
  ORDER BY h.feature, h.created_at DESC;
  ```

  The same list is the sqlc query `ListPaywallHitsByFeature`. For the counts per feature, group
  it: `SELECT feature, count(*) AS hits, count(DISTINCT user_id) AS users FROM paywall_hits
  WHERE created_at >= now() - interval '7 days' GROUP BY feature ORDER BY hits DESC`.

- **Every metered feature enforces on arrival.** auto-apply was the first to ship this
  way: pro's ceiling there is what the tier above is sold on, and one that only counts
  leaves Ultra selling nothing, so shadow mode never applied to it. The other five joined it
  by a later, explicit product decision (subscription-only AI features). Its Ultra fair-use
  figure is deliberately loose and admittedly unmeasured: it guards the HOST, since each
  attempt is a browser, and the feature has run twice in its life.

  **The PRO/ULTRA numbers are still the unread shadow-run reading**: what changed is that
  FREE gets nothing, not that the pro/ultra ceilings themselves were re-verified.

- **Two bounds on tailoring, and they stop different things.** A daily session count bounds
  how many vacancies a candidate works on; a per-session turn ceiling bounds how far one of
  them goes. Measured on production over three August weeks the median session ran 2.7
  turns and one ran 54 — and that account opened two sessions, well inside any daily count,
  while consuming $25 of model calls. Either bound alone leaves the hole this replaced.

  **The turn ceiling binds only an UNPAID tier** (`decideTurn`: every paid tier runs
  unbounded turns), and since the paid gate no unpaid account reaches a turn at all — so in
  production it currently binds nobody. It is kept as mechanism (and tested by lapsing a
  subscription after a session was bought); whether a paid tier should carry one is a
  product decision this package does not make on its own.

- **The turn ceiling is stored nowhere of its own.** It is the HIGHEST slot number the
  session's live charges reach, written as `<session_id>#1`, `#2`, … (`sessionRef`). That
  falls out of the idempotency key rather than fighting it: a bare session id could only
  ever be charged once, which is right for starting a session and wrong for extending one.
  Two simultaneous "continue" clicks attempt the same next reference, they serialise on
  today's counter row, and the second reads as already charged — the race costs one
  allowance, not two, with no lock of its own. **The `#` terminator is load-bearing**:
  without it the scan for `sess-1` sweeps up `sess-12`'s charges.

  **The slot, not the row count.** They differ in the two cases that matter, and both cost
  a candidate a session for nothing. A release voids a row without un-selling the slot, so
  a count would walk the ceiling backwards under a session that already paid. And a session
  granted its first ceiling implicitly (below) holds no row at all, so a count would sell
  its first extension slot 1 — landing it on exactly the ceiling it already had.

- **A turn is the unit charged, not the model call.** One turn averaged 7.1 calls, and that
  number is invisible to whoever asked the question. Charging per call would make two
  identical-looking requests cost differently for reasons nobody can see or influence.

- **A tailoring turn draws on NO assistant allowance.** Charging it twice would let the
  daily assistant allowance decide how deep one CV may be edited. Its refusal names the
  TAILORING feature, because spending another tailoring session is what the candidate has
  to do about it.

- **A session holding no charge gets SLOT 1 implicitly, not zero.** Every conversation open
  on the day this deploys predates the ledger, and treating "no charge" as "never paid for"
  would 402 the next turn of every live one. It is still bounded: one ceiling's worth and
  no more, with the charge taken where sessions are created — the place that can refuse
  before anything exists. Granting it as a SLOT rather than as a bare ceiling is what makes
  the first extension buy slot 2 and therefore buy something; `ceilingsHeld` is the one
  place the grant happens, so the turn rule and the extension price cannot disagree.

- **A pre-check asks `Refuses`, never `Exhausted`.** Exhausted says the allowance is spent;
  Refuses says the allowance is spent AND this feature enforces. A pre-check built on the
  first would hard-refuse while every other surface was still only counting, which is how a
  feature ships enforcing that nobody meant to enforce.

  **This binds the SPA too, which is why `enforced` is on the wire.** Every allowance the
  API returns carries it, and `refuses()` in `web/src/lib/allowance.ts` (and its twin in
  `extension/lib/freehire.ts`) is the only thing a surface may hide or disable an action on.
  A client that blocks on `used >= limit` alone is a wall the server does not have — and it
  suppresses exactly the requests the shadow run is counting, so the numbers the enforcement
  decision rests on come back understated. `Refuses` is a method on the Standing rather than
  on the Store for the same reason: the flag and the rule travel together.

- **Shadow mode: enforcement is a switch, per feature.** With it off, `Consume` records and
  reports but never refuses — the shadow run answers "how many people would this have
  stopped, and where in their day?" against live traffic. Every metered feature ships on
  today (see "Every metered feature enforces on arrival" above), so the switch has nothing
  left to turn on among them — it exists for whatever ships next: `PLAN_ENFORCE` names the
  features whose refusal is on (`match,tailor` or `all`), so a FUTURE feature's shadow run
  can still end one feature at a time without a deploy.

  **A `PLAN_*_DAILY_<F>` suffix writes a dashed feature with an UNDERSCORE**
  (`PLAN_PRO_DAILY_AUTO_APPLY`), while `PLAN_ENFORCE` names the feature exactly as the
  ledger stores it (`auto-apply`). The two spellings differ because only one of them is an
  environment variable NAME: systemd's `EnvironmentFile=` will not parse a dash in one, so a
  suffix taken verbatim from the feature would put the lever out of reach on the only host
  it exists for. The values themselves cannot change — they are in `usage_ledger` and in the
  idempotency index.

- **The fair-use guard belongs to the UNLIMITED paid tiers and ignores the enforcement
  switch.** A tier with a real daily ceiling is already bounded by it, so reaching the guard
  there would mean the allowance was configured above it — a misconfiguration to fix, not a
  caller to accuse of automation. Shadow protects people from ceilings nobody has verified; the guard sits
  ~20× above human behaviour, so what reaches it is automation, and what it protects is the
  gateway rather than a price. It logs when it fires — a guard nobody sees fire gets blamed
  for an outage it did not cause.

  The gateway's own per-key budget is the better home for it eventually (money is the honest
  unit for infrastructure defence), and `internal/ai/llmkey` already mints one. It stays here
  while the pool runs on free keys, which leaves that budget with nothing to calibrate
  against — see the design's "Why the gateway's own budget does not replace this".

- **A release RESTAMPS its row `kind='release'`** rather than deleting it or appending a
  compensating one. `usage_ledger_consume_ref_uniq` is scoped to `kind='consume'`, so:
  appending would leave the reference permanently spent and make the retry free forever;
  deleting would free the reference but erase the fact that a reservation was taken;
  restamping does both jobs, and the day's counter — which sums only live consumptions —
  stays correct.

- **A release gives back the day it took from, not the day it happens on**, and reads that
  day (`GetConsumptionDay`) BEFORE taking any lock. Both matter: a reservation taken at
  23:59 and released at 00:01 would otherwise hand back an allowance never spent today, and
  reading it first keeps `usage_daily` ahead of `usage_ledger` in every path, so a
  consumption and a release for one user cannot deadlock each other.

- **An already-charged reference outranks every ceiling.** A recompute, a retry and a
  reconnect are work already paid for; refusing one charges the candidate for looking at
  their own result twice.

- **Every meter fails open — EXCEPT auto-apply.** A counter that cannot be read logs and
  lets the action through uncharged. Bookkeeping must never refuse a legitimate request, and
  an uncharged turn is a smaller wrong than a candidate stopped by our accounting. The atomic
  consumption is the real ceiling, not the pre-checks in front of it.

  The exception is auto-apply, and the rule above is why it is one. Everywhere else the
  action is a model call whose cost we absorb, so an unmeasured one is a rounding error.
  auto-apply drives a browser and submits a REAL application to a REAL employer under
  somebody's name — an unmeasured one is not ours to absorb, and it is not undoable. The
  honest answer to "we could not tell whether you may" is to say so, so `chargeAutoApply`
  answers 503 both when the charge errors and when the meter is absent. Nothing else in this
  package may copy that without an argument of the same shape.

- **A refusal is 402 and must precede the stream.** Writing it after the headers are out
  makes it an event inside a 200 — invisible to anything checking status codes. Note that
  `c.Status(402).JSON(...)` **returns nil**, so "did it refuse?" cannot be inferred from
  the error; the handler seam returns an explicit bool beside it.

## Placement

The package lives in the `ai` block, not `identity`, and reads `users.pro_until` through
`platform/db`. `identity` sits on the same layer, so a package there could not be called
from `ai/assistant` or `ai/speech` — where two of the metered features live. The rule would
be satisfied and the code impossible.

## What is deliberately absent

`credit_ledger` / `credit_balances` still exist, written by nobody, read only by account
deletion. They are dropped in a later change, once the new path has run long enough that
nobody wants the old numbers back. Migrating their rows was rejected: calling a month's
balance "day one of that month" writes a fact that never happened into an append-only
ledger.

Contributing a board earns nothing here. A one-off act cannot top up a daily allowance, and
banking one for later is the shape the withdrawn balance had. The reward returns in
`add-invites` as days of Pro.
