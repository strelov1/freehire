## Context

`add-pro-subscription` built billing around a premise it stated openly: the provider owns
the subscription, we own one derived timestamp, and the metered path reads a column rather
than an API. That premise is still right. What it did not survive is a second provider.

Three facts set the shape of this change.

**`users.pro_until` has exactly one writer, by design.** `SyncUser` reads Stripe's view of a
customer, reduces it to a timestamp, and writes the column
(`internal/identity/billing/service.go:243`). The spec makes that a requirement, not an
accident: "The plan column has exactly two writers". A second provider writing the same
column turns that guarantee into its opposite — the first Stripe sync after an App Store
purchase finds no Stripe subscription, derives the zero time, and revokes what Apple was
paid for.

**The stores leave no choice about the mechanism.** Apple guideline 3.1.1 and Google Play's
Payments policy require in-app digital content to be sold through in-app purchase.
`freehire-mobile` cannot link out to the Stripe checkout for Pro. RevenueCat is the layer
that turns both stores' receipts into one subscriber record and one webhook.

**The provider contract is already read.** `add-pro-subscription/design.md:180-235` records
RevenueCat's webhook envelope, delivery semantics, signature scheme and subscriber endpoint
as verified on 2026-09-03, because that change was proposed against RevenueCat before it
was implemented against Stripe. Re-verified against the live documentation on 2026-09-04:
the HMAC header, the five-retry schedule at 5/10/20/40/80 minutes, the terminal stop after
those retries, the 60-second response budget and the instruction to deduplicate on event id
all still hold. That research is this change's foundation and is not repeated here.

## Goals / Non-Goals

**Goals:**

- A purchase made in the App Store or Google Play confers Pro on the buying account, and a
  lapse or refund takes it away.
- Two providers coexist such that neither can revoke the other's grant — structurally, not
  by convention.
- Support can grant Pro by hand and have it survive the next provider sync.
- A client can tell where a subscription was bought, because the stores' rules about
  cancellation and double-selling depend on it.
- The lag between a completed store purchase and Pro appearing in `/me/plan` is one
  round-trip, not one webhook delivery.
- Every reader of `users.pro_until` keeps working unchanged.

**Non-Goals:**

- The mobile client. It is a separate change in `freehire-mobile` and consumes the contract
  this one defines.
- Migrating live Stripe subscriptions into RevenueCat, or retiring either provider.
- A cancellation or refund surface of our own. RevenueCat's `management_url` and Stripe's
  portal own that, and this change does not store or expose either.
- Gifted Pro days. `pro_until_granted` is created here because the split has to be complete
  to be safe, but nothing awards it; that is `add-invites`.
- Reconciling a user who holds subscriptions at both providers into a single one. They keep
  both and get the longer reach, which is the correct answer to a question nobody should be
  asked at a paywall.

## Decisions

### `users.pro_until` becomes a stored generated column

Three source columns hold each origin's reach — `pro_until_stripe`,
`pro_until_revenuecat`, `pro_until_granted` — and `pro_until` becomes:

```sql
GENERATED ALWAYS AS (GREATEST(pro_until_stripe, pro_until_revenuecat, pro_until_granted)) STORED
```

`GREATEST` in Postgres skips NULLs and returns NULL only when every argument is NULL, which
is exactly the semantics wanted: an account with no source is free, an account with one is
covered by it, an account with two is covered by the longer.

The property that matters is not that this computes the right answer. It is that the wrong
answer becomes **unwritable**. A provider sync that would erase another's grant is not a bug
that review has to catch; it is a statement Postgres rejects, because `pro_until` cannot be
assigned at all.

*Alternatives considered.* A `billing_entitlements(user_id, provider, pro_until)` table is
more extensible — a third provider needs no migration — but moves the recomputation into
code that can be forgotten at a call site, which is the failure this decision exists to
remove. A `BEFORE UPDATE` trigger avoids the table rewrite but hides the rule where the next
reader of `users` will not see it — and on a table of 1397 rows the rewrite it avoids costs
milliseconds, so it buys nothing for what it hides. Writing `GREATEST` into each provider's `UPDATE`
statement is the cheapest and the worst: it is correct only as long as every future writer
remembers, and it makes a lapse un-expressible, since `GREATEST` of an old value and a new
smaller one never shrinks.

### Existing values are split by `stripe_customer_id`, not guessed

`add-plan-limits` shipped `pro_until` as a hand-set column and `add-pro-subscription` then
pointed Stripe at it, so today's non-NULL values have two origins mixed together. The
migration separates them by the only evidence that exists: an account with a
`stripe_customer_id` (migration 0129) was set by the Stripe sync and its value belongs in
`pro_until_stripe`, where a future cancellation can revoke it. An account without one was
set by hand and belongs in `pro_until_granted`, where no provider will quietly undo it.

Backfilling everything into `pro_until_stripe` would let the next sync revoke support's
manual grants. Backfilling everything into `pro_until_granted` would make every current
Stripe subscriber permanently Pro, including after they cancel. Neither error announces
itself.

The evidence is sound but not total. `stripe_customer_id` is written only when a real Stripe
delivery for that account arrives, so "has a customer id" genuinely means "transacted" — but
an account that transacted **and** then had support hand-set a longer expiry on top is
indistinguishable from one whose expiry Stripe set, and lands in `pro_until_stripe` where the
next sync shortens it. No query can separate those; the migration header carries the
one-line list of candidates instead, which at 1397 accounts is a handful of rows a person
reads once.

### Each provider's near-expiry window reads that provider's own column

The reconciler's second pass finds accounts whose plan is about to lapse. That predicate must
sit on the provider's source column, never on the derived `pro_until`.

It is the first non-obvious consequence of the generated column, and it fails silently in the
direction that costs money. `pro_until` is the *furthest* of three sources, so a Stripe
customer who also holds a store subscription or a manual grant reaching past their renewal
falls outside the window and is never re-checked — and the lost renewal the pass exists to
repair stays lost, on an account that is paying.

### One package, two providers, no new layer entry

The seam is an interface inside `internal/identity/billing`. Reading the package closely
moved where it had to be cut, and the correction is worth recording, because the first
sketch — "verify a signature and read subscriber state" — would have left the largest
difference on the wrong side of it.

**What actually differs most is how an account is addressed.** Stripe stores a customer id
(`users.stripe_customer_id`, migration 0129) and resolves an account by two routes, because
a first purchase arrives before any binding exists. RevenueCat has none of that: `app_user_id`
IS `users.id`, so there is no second identifier, nothing to bind, and nothing to repair
before a sync. A seam that hid only the signature and the state read would have left that
asymmetry as a branch inside the shared code — the awkward special case `AGENTS.md` says to
reshape rather than bolt on.

So the interface is: name, enabled, signature header, accept, account, bind, reach, store,
dueSoon. It hides addressing, HTTP shape, entitlement rules and column names, and `bind` is
honestly a no-op for RevenueCat rather than a stub.

**What the interface deliberately excludes is the other half of the correction.** Checkout,
a management portal, prices and receipts have no RevenueCat counterpart at all — a store
subscription is bought, changed, cancelled and refunded inside the App Store or Google Play,
where we have no API and no business having one. Putting them behind the seam would give
RevenueCat four methods that can only answer "not applicable", and an interface whose
implementations refuse half of it is a union type wearing an abstraction's clothes.

The package therefore splits in two: an unexported `engine` holding everything true of both
providers (accept, record, apply, sync, the two reconciler passes), and the exported
`Service` — Stripe — embedding it and adding the web-only surface. RevenueCat gets its own
type over the same engine. The split is not invented: `cmd/billing-sync` already calls
exactly the engine's methods and nothing else, while the Stripe HTTP handler calls the rest.

Keeping it in one package is deliberate. `AGENTS.md` requires a new package to be added to
the table in `internal/platform/arch/layering/blocks.go`, and a package that exists only to
hold one provider's HTTP client earns neither the entry nor the import graph it adds. The
seam is an interface, which is the smallest thing that expresses "two of these".

`verifySignature` is already provider-agnostic: it verifies `t=<unix>,v1=<hex>` over
`"<t>.<raw body>"` with a freshness window, and RevenueCat signs in precisely that scheme.
Only `SignatureHeader` becomes per-provider. The function moves, unchanged, behind the seam.

### Entitlement is read from RevenueCat v1, and derived like Stripe's

`GET https://api.revenuecat.com/v1/subscribers/{app_user_id}` with the secret key returns an
entitlements map. The configured entitlement id (`REVENUECAT_ENTITLEMENT`, default `pro`)
selects one, and its reach is the later of `expires_date` and
`grace_period_expires_date` — the same reasoning that puts Stripe's `past_due` among the
entitling statuses. A subscriber mid-retry has not stopped paying, and the period they
already bought still bounds it.

Two traps from the recorded research are load-bearing, and both are handled in the direction
that fails safely rather than generously:

- **`expires_date` is nullable and null means non-expiring.** We sell no lifetime product,
  so this case should never arrive. It is handled anyway, because a case nobody will notice
  being handled wrongly is the one worth handling correctly. A null reads as
  non-expiring only when the entitlement is otherwise present and current; an entitlement we
  cannot read at all entitles nobody, exactly as `entitlement.go` already argues for a
  subscription whose period end will not parse.
- **The GET creates the subscriber when the id is unknown** — a read with a write's
  consequence. A pass that can reach accounts in BULK asks `knows()` first, so no subscriber
  is manufactured by the reconciler or by an event replay.

  **The check binds the pass, not the read**, and getting that backwards cost a whole feature
  in an earlier draft. With the check inside the read, the sync route inherited it — and a
  first-time buyer has no recorded event and a NULL column, because that is what "first"
  means. All three recovery paths refused them at once, so a purchase whose delivery was lost
  would never have conferred anything. One authenticated, rate-limited caller asking about
  their own id is not a bulk pass: the provider's device SDK created that subscriber before
  the app ever reached us.

*Alternative considered.* The v2 API exposes `gives_access`, the provider's own answer to
"does this entitle", which is philosophically closer to this package's rule against keeping a
second copy of a provider's state machine. It is not chosen here: v2 needs a project
identifier and a differently-scoped key, spends more calls, and would leave two client
shapes in one package for no behaviour we need. The derivation from expiry is three lines
and mirrors what Stripe's path already does. Worth revisiting if a second entitlement or a
non-subscription product ever appears.

### `POST /billing/revenuecat/sync` closes the purchase lag

A store purchase completes on the device and the webhook arrives afterwards. A paywall that
says "please wait" to someone who has just paid is a support ticket, and if the delivery is
one of the ones that never arrives, the wait is up to an hour until the reconciler runs.

The route takes the caller's own identity from the session, re-reads their RevenueCat state
and writes `pro_until_revenuecat`. It carries no body and names no user: a route that
accepted a user id would be a way to ask us to call a third party once per request, per
name. It is rate-limited per caller for the same reason.

It is the same operation the reconciler performs, reached by a different trigger — not a
second path to the column. That is what keeps it from becoming a second source of truth.

### `pro_source` names the origin that currently confers Pro

`GET /me/plan` gains `pro_source`, one of `stripe`, `revenuecat` or `granted`, present only
when `pro_until` is in the future — computed from which source column equals the derived
value, with ties broken in that order.

It exists because the stores' rules make the origin behavioural, not informational. Apple
forbids directing an in-app subscriber to a web page to cancel. Offering an in-app purchase
to someone already paying through Stripe sells them the same thing twice, and RevenueCat
would happily take the second payment.

The tie-break order is stated rather than left to whichever column is read first, so the
answer is stable across deployments. A tie means two sources reach the same instant, which
is rare and harmless; naming Stripe first makes the client's advice about cancellation point
at the surface the user most likely used to buy.

### Billing stays inert unless configured, per provider

`REVENUECAT_API_KEY`, `REVENUECAT_WEBHOOK_SECRET` and `REVENUECAT_ENTITLEMENT` (defaulted)
govern the RevenueCat provider alone. Absent them its routes are not mounted and
`cmd/billing-sync` skips its pass, while Stripe keeps working — the two configurations are
independent, because a deployment that sells only on the web is a legitimate one and so is a
self-hoster who sells nothing.

## Risks / Trade-offs

**The migration rewrites `users`.** → Dropping `pro_until` and adding a stored generated
column are both table rewrites under an `ACCESS EXCLUSIVE` lock, and both are unavoidable for
this shape. The cost is small and this is measured rather than assumed: `users` held **1397
rows** on prod on 2026-09-03, recorded in `migrations/0129_users_stripe_customer_id.sql` as
"measured on prod, not estimated". At that size the rewrite is milliseconds.

An earlier draft of this document called it 8 million rows, taking the figure from
`migrations/0120_users_pro_until.sql`, which speaks of 8 million accounts and cites no
measurement. 0129 measured 1397 three days later. Where an unmeasured figure and a measured
one disagree, the measured one is what to plan against. The correction matters in one
direction only — it removes the strongest argument for the trigger
alternative, which was that a rewrite here would need a maintenance window. It does not need
one. It still runs manually on prod before the deploy, for the ordering reason below rather
than for its duration.

**`squawk` will object to the migration** (`pnpm check:sql` gates added files). → Expected,
and answered inline: each objection is either genuinely accepted with a
`-- squawk-ignore <rule>` and its reason on the line above, or the statement is reshaped.
An unexplained ignore is not acceptable; the reason is the point.

**A RevenueCat retry may fall outside the five-minute signature window.** → Stripe re-signs
each retry, which is why `signatureWindow` can be narrow. Whether RevenueCat re-signs is not
stated in its documentation, and its last retry arrives 80 minutes after the first. If
retries carry the original signature, every retry is rejected and the reconciler becomes the
only path — silently. This is verified against a real delivery before the change is
considered done, and the window is per-provider so widening it for RevenueCat costs Stripe
nothing. Listed again as an open question because it cannot be settled by reading.

**`sqlc` must accept a generated column.** → It is read like any other column and every
existing query keeps compiling; what breaks is `SetProUntil`, which this change removes
anyway. The codegen is re-run and the build is the check.

**Two providers can bill one account simultaneously.** → We take both and grant the longer
reach, and `pro_source` lets the client avoid setting it up in the first place. Refunding the
redundant one is a support action at whichever store took it, which is where the money is.

**A stored generated column cannot be set by hand.** → That is the intent, and it is why
`pro_until_granted` exists. Support's `UPDATE` moves one column to the left; the runbook in
`deploy/` says so, because the old statement will now fail and the failure should teach
rather than puzzle.

## Migration Plan

1. Add the migration. Verify on a fresh initdb volume that a clean install produces the
   generated column, and separately against a copy of prod that the split by
   `stripe_customer_id` lands every existing non-NULL value in exactly one source column.
2. Merge. **The release runs it** — `deploy/bin/release.sh` builds `cmd/migrate` and runs it
   before the new colour starts, so that colour never serves against an older schema, and a
   migration failure aborts the release with the live colour untouched. An earlier draft of
   this plan called for a manual run first; that was written from a wrong model of the fleet,
   and following it would have meant doing by hand what the pipeline already does under a
   lock.

   The one gap it leaves is between the migration and the flip, where the OLD binary briefly
   serves against the NEW schema. Its only write to this column, `SetProUntil`, fails with
   428C9 — closed, not wrong: a Stripe delivery in that window goes unacknowledged and is
   retried, and the reconciler repeats a sync that changes nothing.
3. Stripe now writes `pro_until_stripe`; behaviour is unchanged.
4. Configure RevenueCat in the environment and register the webhook. Until that moment the
   RevenueCat routes 404 and nothing else differs.
5. Verify with a sandbox purchase end to end, including a cancellation and a refund.

**Rollback.** Between steps 2 and 3 there is a window where deployed code writes a column it
can no longer assign — `pro_until` still exists and still reads correctly, it has become
generated. The write fails closed with `428C9` rather than storing something wrong, which is
why the ordering is migrate-then-deploy and not the reverse.

After step 3, rollback means redeploying the previous binary, which needs `pro_until`
writable — so `deploy/rollback/0135_pro_until_sources.down.sql` restores it as a plain column
seeded from `GREATEST` of the three. **It keeps the three source columns rather than dropping
them**, and that is the load-bearing part: the old binary neither reads nor writes them, so
they cost it nothing, while dropping them would destroy the only record of which origin
conferred each plan. Re-applying the forward migration over that wreckage would re-split
every account by `stripe_customer_id` and move money in both directions — a store subscriber
with no Stripe customer becomes an unrevocable grant that no refund can take back, and a
Stripe customer's longer manual grant becomes revocable and is shortened by the next sync.

Rolling forward again is therefore `0135_pro_until_sources.reapply.sql`, which only restores
the generated column, and explicitly not the migration a second time. Both files are executed
from disk by the integration tests, including the full round trip, so the pair is verified
rather than asserted.

## Open Questions

- **Does RevenueCat re-sign each webhook retry?** Decides whether the RevenueCat signature
  window can be five minutes or must span the 80-minute retry tail. Answered by inspecting a
  real retried delivery in the dashboard, not by reading. Until it is answered the window is
  provisionally wide enough to admit the last retry, which is the safe direction: a
  too-narrow window drops paid subscriptions, a too-wide one lengthens the life of a captured
  delivery whose replay is already idempotent by event id.
- **Which store product identifiers back the entitlement?** They are created in App Store
  Connect and Play Console and mapped in RevenueCat, not configured here — this change binds
  to the entitlement id, not to products. Named as an open question because the mobile change
  and the dashboard setup have to agree on them, and nothing in either repository will catch
  a mismatch.

## Settled

**HMAC signing is required, and there is no fallback.** This was drafted as an open question —
whether the integration offers signing at all, and what to do if it offers only the shared
`Authorization` header. It is decided: only the signature is accepted. The header alternative
is a bearer credential that anyone who sees one delivery can replay forever, and the whole
point of the freshness window is that a captured delivery stops being useful. If the dashboard
turns out not to offer signing, that is a reason to stop and reconsider the integration — not
a reason to accept the weaker scheme, and no code path exists for it.

**The entitlement id is `pro`**, which is `REVENUECAT_ENTITLEMENT`'s default. It is a string
contract between the dashboard and this deployment: get it wrong and every store subscriber
reads as free, which looks exactly like nobody having bought anything.
