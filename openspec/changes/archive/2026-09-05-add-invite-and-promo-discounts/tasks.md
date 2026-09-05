## 1. Schema

- [x] 1.1 Write `migrations/0140_promo_and_invites.sql`: `promo_codes` (code PK upper-cased by
  a check, `percent_off` smallint 1..100, `max_uses` int NULL, `uses` int NOT NULL DEFAULT 0,
  `expires_at` timestamptz NULL, `active` bool NOT NULL DEFAULT true, `note` text,
  `created_at`), `promo_redemptions` (`user_id` UNIQUE — one code per account for life —
  `code` FK, `redeemed_at`), `invite_codes` (`user_id` PK, `code` UNIQUE, `created_at`),
  `invite_rewards` (`id`, `referrer_id`, `referee_id` UNIQUE, `status`, `amount_cents`,
  `granted_at`, `delivered_at`, `created_at`, CHECK `referrer_id <> referee_id`). Verify with
  `pnpm check:sql`.
- [x] 1.2 Add `internal/platform/db/queries/promo.sql`: the atomic seat claim
  (`UPDATE … WHERE … RETURNING`), redemption insert, preview read, invite-code upsert and
  lookup, reward insert, pending-rewards-for-payable-invitees, granted-count-per-referrer,
  accrued-undelivered-for-user, and the two stamps (granted, delivered). Run `make sqlc` and
  commit the generated diff.

## 2. The promo package

- [x] 2.1 Register `internal/identity/promo` in the layering table at
  `internal/platform/arch/layering/blocks.go` (identity, layer 3) and confirm the arch test
  passes with `-tags=integration`.
- [x] 2.2 `promo.Service`: `Preview(ctx, code)` — read-only validity, no side effects — and
  `Redeem(ctx, userID, code)` returning the percentage or a typed refusal
  (`ErrUnknownCode`, `ErrCodeExhausted`, `ErrAlreadyRedeemed`). Refusals must not distinguish
  "no such code" from "not eligible" in what reaches a caller.
- [x] 2.3 Invite codes: `LinkFor(ctx, userID)` mints on first ask from `crypto/rand`, is
  idempotent, and never rotates. Uniqueness comes from the constraint, not from a read.
- [x] 2.4 `Attribute(ctx, refereeID, code)`: writes a `pending` reward, refuses self-referral,
  and treats an unknown code and a duplicate invitee as no-ops rather than errors.
- [x] 2.5 `Stats(ctx, userID)`: invitee count, rewarded count, total credit earned. No invitee
  identity in the result type at all — not filtered at the edge.
- [x] 2.6 `PendingDiscount(ctx, userID)`: the one percentage an account gets on a checkout —
  a redeemed promo code or a pending invite — and which of the two it was.
- [x] 2.7 Reward-ceiling config: `INVITE_REWARD_MAX_PER_USER`, default 12; an unparseable or
  non-positive value logs and falls back rather than failing.

## 3. Billing: the two provider abilities

- [x] 3.1 Give `client.do` an optional header argument and thread it through the existing call
  sites unchanged.
- [x] 3.2 `client.createCoupon` (`duration=once`, `percent_off`) with an idempotency key
  derived from account, code and price; attach it to the session as `discounts[0][coupon]`.
- [x] 3.3 Add `billing.Discount` (a percentage, nothing else) and the parameter on
  `Service.CheckoutURL`. A zero `Discount` must produce a byte-identical request to today's.
- [x] 3.4 `client.creditCustomerBalance` → `POST /v1/customers/{id}/balance_transactions` with
  a negative amount and `Idempotency-Key` = the reward id.
- [x] 3.6 `Service.CreditAccount(ctx, userID, cents, key)`: resolve the account's customer,
  creating and binding one through the write-once setter when there is none, then credit it.
  Assert no second customer is created for an already-bound account.
- [x] 3.5 `client.hasCollectedPayment(ctx, customerID)`: any invoice with `amount_paid > 0`.
  Assert against a stub that an active-but-uncollected subscription answers false.

## 4. Granting the reward

- [x] 4.1 Add the third pass to `cmd/billing-sync`: pending rewards whose invitee holds a
  `stripe_customer_id` → ask the provider → grant at 50% of list price, resolved at grant time
  and stored. Bounded by `INVITE_REWARD_MAX_PER_RUN`.
- [x] 4.2 Deliver every granted, undelivered reward through `Service.CreditAccount`. Assert
  the pass is a no-op on a second run, and that a referrer who has never bought gets a
  customer created for them rather than being skipped.
- [x] 4.3 Enforce the per-referrer ceiling: over it, the row stays `pending`, nothing is
  credited, and the invitee's own discount is untouched.
- [x] 4.4 Assert a store-only invitee (RevenueCat, no Stripe customer) never grants. This is
  a property of the query's `stripe_customer_id IS NOT NULL` filter, so the assertion needs a
  real database and lands with the integration tests in 8.2.

## 5. HTTP

- [x] 5.1 `GET /api/v1/me/invite` → link, counts, accrued credit. Behind `RequireAuth`.
- [x] 5.2 `POST /api/v1/me/promo/preview` behind `RequireAuth` plus
  `ratelimit.Middleware(…, ratelimit.KeyByUserOrIP("promo"), …)`. Assert 401 anonymous, 429
  over the limit, and that a preview consumes no seat.
- [x] 5.3 Accept an optional `code` on the existing checkout route: redeem, resolve the single
  discount, pass it to `billing`, and state in the response which discount was applied.
- [x] 5.4 Read the attribution cookie in the password registration handler and call
  `promo.Attribute`, expiring the cookie on success. A failure there must never fail a
  registration.
- [x] 5.5 Do the same in the OAuth callback — the majority signup path, and the one that
  returns on a bodyless GET redirect, so the cookie is the only thing that can carry the code.
  Assert with a test that an OAuth signup attributes.

## 6. Web

- [x] 6.1 Capture `?ref=` in `web/src/hooks.server.ts` on any request: set an httpOnly,
  `SameSite=Lax`, `Secure`, 30-day cookie, and only when one is not already present (first
  toucher wins). Unit-test the capture rule, including that a second code does not overwrite.
- [x] 6.2 `web/src/routes/r/[code]/+server.ts`: the short form of the same link — set the
  cookie and redirect to `/`.
- [x] 6.3 Capture `?promo=` the same way into its own, non-httpOnly cookie, and read it in the
  pricing page's server `load` to prefill the field.
- [x] 6.4 Promo-code field on `web/src/routes/pricing/+page.svelte`, wired to preview then
  checkout, showing the refusal text the API returns.
- [x] 6.5 Invite page: the account's link with copy-to-clipboard, invitee and paid counts, and
  accrued credit. Follow the existing account-section routing and its auth redirect gate.
- [x] 6.6 `pnpm --dir web lint` and `pnpm --dir web test` green (a fresh worktree needs
  `svelte-kit sync` first).

## 7. Guards and docs

- [x] 7.1 A test that walks the module for a string literal shaped like a live promo code and
  fails naming the file. Fixtures use an allowlisted prefix so the guard cannot be satisfied
  by weakening it.
- [x] 7.2 `internal/identity/promo/AGENTS.md`: the scope, why it is not `engage/referral`, why
  granting is worker-side, and the one-discount-per-session rule.
- [x] 7.3 Note the new capability in `internal/identity/billing/AGENTS.md` — a `Discount`
  parameter it executes and never interprets — without widening the stated scope.
- [x] 7.4 Record `INVITE_REWARD_MAX_PER_USER` and `INVITE_REWARD_MAX_PER_RUN` where the other
  worker variables are documented in `CLAUDE.md`.

## 8. Verification

- [x] 8.1 `gofmt -l .` silent, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`.
- [x] 8.2 `go test -tags=integration ./internal/identity/promo/ ./internal/identity/billing/`,
  including the two assertions that need a real database: `CreditAccount` creates no second
  customer for an already-bound account, and the seat claim cannot be won twice.
- [x] 8.3 `pnpm check:sql`, `pnpm check:links`, `golangci-lint run`.
- [x] 8.4 Walk the whole flow against a Stripe stub: invited signup → discounted checkout →
  paid invoice → worker grants → referrer credited; then re-run the worker and assert nothing
  moves.
