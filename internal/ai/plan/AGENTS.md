# Plan and allowance conventions

## Scope
The plan a user is on and the per-day allowance each metered AI feature draws from. One
question in, yes or no out: `Consume(user, feature, ref)`. Which plan, what it allows, how
much is left and whether a refusal is switched on are all this package's business.

The rule itself is a pure function (`decide`, `decideTurn`) of the plan, the feature, what
the day already holds and whether the reference was charged before — so every branch is
tested without a database, including the ones that are awkward to reach through a
transaction. `store.go` is the transaction around it and nothing else.

## Always true

- **Two bounds on tailoring, and they stop different things.** A daily session count bounds
  how many vacancies a candidate works on; a per-session turn ceiling bounds how far one of
  them goes. Measured on production over three August weeks the median session ran 2.7
  turns and one ran 54 — and that account opened two sessions, well inside any daily count,
  while consuming $25 of model calls. Either bound alone leaves the hole this replaced.

- **The turn ceiling is stored nowhere of its own.** It is the count of charges the session
  holds, written as `<session_id>#1`, `#2`, … (`sessionRef`). That falls out of the
  idempotency key rather than fighting it: a bare session id could only ever be charged
  once, which is right for starting a session and wrong for extending one. Two simultaneous
  "continue" clicks attempt the same next reference, the index admits one, and the second
  reads as already charged — the race costs one allowance, not two, with no lock of its own.
  **The `#` terminator is load-bearing**: without it the count for `sess-1` sweeps up
  `sess-12`'s charges.

- **A turn is the unit charged, not the model call.** One turn averaged 7.1 calls, and that
  number is invisible to whoever asked the question. Charging per call would make two
  identical-looking requests cost differently for reasons nobody can see or influence.

- **A tailoring turn draws on NO assistant allowance.** Charging it twice would let the
  daily assistant allowance decide how deep one CV may be edited. Its refusal names the
  TAILORING feature, because spending another tailoring session is what the candidate has
  to do about it.

- **A session holding no charge gets ONE ceiling, not zero.** Every conversation open on
  the day this deploys predates the ledger, and treating "no charge" as "never paid for"
  would 402 the next turn of every live one. It is still bounded: one ceiling's worth and
  no more, with the charge taken where sessions are created — the place that can refuse
  before anything exists.

- **A pre-check asks `Refuses`, never `Exhausted`.** Exhausted says the allowance is spent;
  Refuses says the allowance is spent AND this feature enforces. A pre-check built on the
  first would hard-refuse while every other surface was still only counting, which is how a
  feature ships enforcing that nobody meant to enforce.

- **Shadow mode: enforcement is a switch, per feature, and it ships off.** With it off,
  `Consume` records and reports but never refuses — the shadow run answers "how many people
  would this have stopped, and where in their day?" against live traffic. `PLAN_ENFORCE`
  names the features whose refusal is on (`match,tailor` or `all`), so the run ends one
  feature at a time without a deploy.

- **The fair-use guard belongs to the PRO plan and ignores the enforcement switch.** A free
  account is already bounded by its daily allowance, so reaching the guard there would mean
  the allowance was configured above it — a misconfiguration to fix, not a caller to accuse
  of automation. Shadow protects people from ceilings nobody has verified; the guard sits
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

- **Every meter fails open.** A counter that cannot be read logs and lets the action
  through uncharged. Bookkeeping must never refuse a legitimate request, and an uncharged
  turn is a smaller wrong than a candidate stopped by our accounting. The atomic
  consumption is the real ceiling, not the pre-checks in front of it.

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
