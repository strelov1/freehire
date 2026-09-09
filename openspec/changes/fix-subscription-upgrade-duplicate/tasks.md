## 1. Provider client: name and modify an existing subscription

- [x] 1.1 Add `ID` (subscription id) and the first item's `ID` to
      `stripeSubscriptionList` in `internal/identity/billing/client.go`, and carry both onto
      the parsed `subscription` (new fields `ID`, `ItemID` in `entitlement.go`).
- [x] 1.2 Add `client.updateSubscriptionPrice(ctx, subscriptionID, itemID, priceID string) error`
      calling `POST /v1/subscriptions/{id}` with `items[0][id]`, `items[0][price]`, and
      `proration_behavior=create_prorations`.
- [x] 1.3 Unit test `updateSubscriptionPrice` against the `testClient` stub (assert path and
      form fields), following the `TestClientCheckoutSession` pattern.
- [x] 1.4 Unit test that `subscriberState` now parses the subscription id and the first item's
      id from a fixture response (extend `TestClientSubscriberState`).

## 2. Pure decision: what CheckoutURL should do

- [x] 2.1 In `entitlement.go`, add a pure function deciding, from a customer's current
      `subscriber` and a requested price id, whether to: do nothing (already on that price),
      update an existing subscription in place (returning its subscription id + item id), or
      open a new checkout (no entitling subscription at all) — searching across both
      `proPrices` and `ultraPrices` combined, reusing `bestEntitling`.
- [x] 2.2 Table-driven unit tests for it in `entitlement_test.go`, covering: no subscription
      (→ new checkout), an active Pro subscription requesting Ultra (→ update in place), an
      active Ultra subscription requesting Pro (→ update in place), a request for the price
      already held (→ no-op), and a customer already holding two entitling subscriptions
      (→ update in place picks the best-entitling one, per design.md's stated Non-Goal).

## 3. Wire it into CheckoutURL

- [x] 3.1 In `Service.CheckoutURL` (`service.go`), when an existing Stripe customer id is
      found, read `client.subscriberState`, apply the task 2.1 decision, and branch: no-op
      returns `s.cfg.ReturnURL()`; update-in-place calls `updateSubscriptionPrice` and returns
      `s.cfg.ReturnURL()`; no entitling subscription falls through to the existing
      `createCheckoutSession` call unchanged.
- [x] 3.2 A failure reading `subscriberState` on this path returns an error (never falls back
      to opening a new checkout) — this is the one line the bug fix depends on, so make it
      impossible to miss in review: comment why, next to the `if err != nil`.
- [x] 3.3 Update `CheckoutURL`'s doc comment to describe the new upgrade/downgrade-in-place
      behavior, not just "buys Pro".

## 4. Billing overview: surface an existing duplicate

- [x] 4.1 Add `MultipleSubscriptions bool` (`json:"multiple_subscriptions,omitempty"`) to
      `Overview` in `overview.go`.
- [x] 4.2 In `overviewFor`, compute it from the count of entitling SUBSCRIPTIONS across both
      price lists (`moreThanOneEntitling`; counting subscriptions rather than price ids is
      what keeps a legitimate one-subscription-two-items case from a false positive) — more
      than one sets the flag — without changing which subscription `best`/the rest of the
      response describes.
- [x] 4.3 Unit tests in `overview_test.go` extending the `serviceWithProvider`/`subscribedTo`
      pattern: one subscription → flag false/absent; two concurrent entitling subscriptions
      (the reported bug shape: Pro + Ultra both active) → flag true, while `amount_cents`/
      `renews_at` still describe the best-entitling one exactly as before.

## 5. Frontend

- [x] 5.1 Add `multiple_subscriptions?: boolean` to the `BillingOverview` type
      (`web/src/lib/types.ts` or wherever it is declared).
- [x] 5.2 In `PlanView.svelte`, when `billing.multiple_subscriptions` is true, show a warning
      in the subscription section pointing at "Manage subscription" (the existing
      `manageUrl` link) to resolve the duplicate — add the copy to `PlanView.messages`
      alongside the existing strings.
- [x] 5.3 Confirmed (read the code, no change needed): `pricing/+page.svelte`'s
      `buy()`/`buyUltra()` and `api.ts`'s `billingCheckout` need no changes — both already just
      follow whatever URL `/billing/checkout` returns, which is exactly what makes the backend
      change alone sufficient — `window.location.href = url` lands on `/my/plan` either way.
      `svelte-check` (0 errors) and `eslint` (clean) confirmed on the changed frontend files.

## 6. Verify

- [x] 6.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...` — all green (see session log:
      full `go test ./...` run, every package `ok`).
- [x] 6.2 `go vet -tags=integration ./...` — clean.
- [ ] 6.3 Manually confirm in a Stripe test-mode account: subscribe to Pro, then click
      "Upgrade to Ultra" on `/pricing` while signed in — assert via the Stripe dashboard that
      the SAME subscription now carries the Ultra price (one subscription, one item, price
      swapped), not a second subscription. **Not run in this session** — no Stripe test-mode
      credentials or database available in the sandboxed environment; needs a human (or a
      staging deploy) with `STRIPE_SECRET_KEY` pointed at a Stripe test-mode account. Partially
      substituted by 7.5 below (a real-Postgres, stubbed-Stripe integration test exercising
      the same path), which is a lower bar than a real Stripe test-mode account.

## 7. Code review follow-ups

A first review (before this section existed) found the core architecture sound but flagged
two ways the new logic could still silently reopen a duplicate/mis-billed subscription given
unusual subscription data, plus a response-contract gap. Addressed here rather than filed as
follow-up issues, since the whole point of this change is closing exactly those paths.

- [x] 7.1 **Important:** a subscription carrying more than one item (a customer who upgraded
      through Stripe's own Customer Portal, which can add a new item to an existing
      subscription rather than replace one) must not have its price replaced by silently
      guessing item[0] — the requested price might belong to a different item entirely. Added
      `checkoutTarget.Ambiguous`; `decideCheckoutTarget` returns it when the selected
      subscription's `PriceIDs` has more than one entry, and `CheckoutURL` turns that into an
      error rather than a checkout or an update. Table-driven cases added to
      `TestDecideCheckoutTarget`; integration coverage in `TestCheckoutURLRefusesToGuessOnAnAmbiguousSubscription`.
- [x] 7.2 **Important (documented, not changed):** `decideCheckoutTarget` reuses
      `bestEntitling`'s existing, deliberate "a subscription whose period end cannot be read
      entitles nobody" rule (see the big comment on the `subscription` type and
      `TestProUntilFrom`). A selection of its own for checkout purposes would let `CheckoutURL`
      believe a customer is entitled while the rest of billing (`SyncUser`, the plan
      derivation) believes they are not — a worse inconsistency than opening a checkout for an
      account the whole system already treats as unentitled. Pinned with a dedicated test case
      and a comment explaining why, rather than left as an unstated assumption.
- [x] 7.3 **Important:** `CheckoutURL` computed a discount and the `/billing/checkout` handler
      always reported it as applied, even when the update-in-place branch took it and
      (correctly, per design.md) dropped it. `CheckoutURL` now returns the Discount that was
      actually applied — `Discount{}` for every in-place-update/no-op branch, the input
      `discount` only when a Checkout Session coupon was actually minted — and the handler
      reports that instead of the input. Covered by
      `TestCheckoutURLUpdatesAnExistingSubscriptionInPlace` (discount dropped, no coupon call)
      and `TestCheckoutURLOpensACheckoutSessionForAKnownCustomerWithNoSubscription` (discount
      applied, coupon minted).
- [x] 7.4 **Minor:** `updateSubscriptionPrice` now carries a fresh idempotency key per call
      (`uuid.NewString()`, already a module dependency) — it creates a proration invoice item,
      and without a key a low-level HTTP transport retry of the same request could bill the
      same change twice. Test extended in `client_test.go`.
- [x] 7.5 **Minor:** added `checkout_integration_test.go` (real Postgres via testcontainers,
      stubbed Stripe) exercising `CheckoutURL` end to end — the one piece of the fix
      (`GetStripeCustomerID` → `decideCheckoutTarget` → `updateSubscriptionPrice`/
      `createCheckoutSession` wiring in `service.go`) the pure unit tests cannot reach, per
      design.md's own stated reason `CheckoutURL` has no dedicated unit test. Three tests: an
      existing entitling subscription is updated in place and no coupon/checkout call is made;
      a known customer with no entitling subscription still reaches an ordinary checkout with
      its discount applied; an ambiguous (multi-item) subscription is refused rather than
      guessed at.
- [x] 7.6 **Minor:** fixed `PlanView.messages.ts`'s `duplicateWarning` copy — it said
      "...below" but the "Manage or cancel" link it refers to renders above the subscription
      section, not below it. Reworded to be layout-independent (en + ru).
