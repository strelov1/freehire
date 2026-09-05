## 1. Schema

- [ ] 1.1 Write `migrations/0140_promo_and_invites.sql`: `promo_codes` (code PK upper-cased by
  a check, `percent_off` smallint 1..100, `max_uses` int NULL, `uses` int NOT NULL DEFAULT 0,
  `expires_at` timestamptz NULL, `active` bool NOT NULL DEFAULT true, `note` text,
  `created_at`), `promo_redemptions` (`user_id` UNIQUE — one code per account for life —
  `code` FK, `redeemed_at`), `invite_codes` (`user_id` PK, `code` UNIQUE, `created_at`),
  `invite_rewards` (`id`, `referrer_id`, `referee_id` UNIQUE, `status`, `amount_cents`,
  `granted_at`, `delivered_at`, `created_at`, CHECK `referrer_id <> referee_id`). Verify with
  `pnpm check:sql`.
- [ ] 1.2 Add `internal/platform/db/queries/promo.sql`: the atomic seat claim
  (`UPDATE … WHERE … RETURNING`), redemption insert, preview read, invite-code upsert and
  lookup, reward insert, pending-rewards-for-payable-invitees, granted-count-per-referrer,
  accrued-undelivered-for-user, and the two stamps (granted, delivered). Run `make sqlc` and
  commit the generated diff.

## 2. The promo package

- [ ] 2.1 Register `internal/identity/promo` in the layering table at
  `internal/platform/arch/layering/blocks.go` (identity, layer 3) and confirm the arch test
  passes with `-tags=integration`.
- [ ] 2.2 `promo.Service`: `Preview(ctx, code)` — read-only validity, no side effects — and
  `Redeem(ctx, userID, code)` returning the percentage or a typed refusal
  (`ErrUnknownCode`, `ErrCodeExhausted`, `ErrAlreadyRedeemed`). Refusals must not distinguish
  "no such code" from "not eligible" in what reaches a caller.
- [ ] 2.3 Invite codes: `LinkFor(ctx, userID)` mints on first ask from `crypto/rand`, is
  idempotent, and never rotates. Uniqueness comes from the constraint, not from a read.
- [ ] 2.4 `Attribute(ctx, refereeID, code)`: writes a `pending` reward, refuses self-referral,
  and treats an unknown code and a duplicate invitee as no-ops rather than errors.
- [ ] 2.5 `Stats(ctx, userID)`: invitee count, paid count, accrued undelivered credit. No
  invitee identity in the result type at all — not filtered at the edge.
- [ ] 2.6 `PendingDiscount(ctx, userID)`: resolves the one discount an account gets, credit
  before percentage, and reports which it chose.
- [ ] 2.7 Reward-ceiling config: `INVITE_REWARD_MAX_PER_USER`, default 12; an unparseable or
  non-positive value logs and falls back rather than failing.

## 3. Billing: the two provider abilities

- [ ] 3.1 Give `client.do` an optional header argument and thread it through the existing call
  sites unchanged.
- [ ] 3.2 `client.createCoupon` (`duration=once`, percent or amount+currency) with an
  idempotency key derived from account, code and price; attach it to the session as
  `discounts[0][coupon]`.
- [ ] 3.3 Add `billing.Discount` and the parameter on `Service.CheckoutURL`. A zero `Discount`
  must produce a byte-identical request to today's.
- [ ] 3.4 `client.creditCustomerBalance` → `POST /v1/customers/{id}/balance_transactions` with
  a negative amount and `Idempotency-Key` = the reward id.
- [ ] 3.5 `client.hasCollectedPayment(ctx, customerID)`: any invoice with `amount_paid > 0`.
  Assert against a stub that an active-but-uncollected subscription answers false.

## 4. Granting the reward

- [ ] 4.1 Add the third pass to `cmd/billing-sync`: pending rewards whose invitee holds a
  `stripe_customer_id` → ask the provider → grant at 50% of list price, resolved at grant time
  and stored. Bounded by `INVITE_REWARD_MAX_PER_RUN`.
- [ ] 4.2 Deliver a granted reward as a balance credit when the referrer has a customer; leave
  it undelivered when they do not. Assert the pass is a no-op on a second run.
- [ ] 4.3 Enforce the per-referrer ceiling: over it, the row stays `pending`, nothing is
  credited, and the invitee's own discount is untouched.
- [ ] 4.4 Assert a store-only invitee (RevenueCat, no Stripe customer) never grants.

## 5. HTTP

- [ ] 5.1 `GET /api/v1/me/invite` → link, counts, accrued credit. Behind `RequireAuth`.
- [ ] 5.2 `POST /api/v1/me/promo/preview` behind `RequireAuth` plus
  `ratelimit.Middleware(…, ratelimit.KeyByUserOrIP("promo"), …)`. Assert 401 anonymous, 429
  over the limit, and that a preview consumes no seat.
- [ ] 5.3 Accept an optional `code` on the existing checkout route: redeem, resolve the single
  discount, pass it to `billing`, and state in the response which discount was applied.
- [ ] 5.4 Read the attribution cookie in the registration handler and call
  `promo.Attribute`. A failure there must never fail a registration.

## 6. Web

- [ ] 6.1 `web/src/routes/r/[code]/+server.ts`: set the httpOnly, `SameSite=Lax`, 30-day
  cookie and redirect to `/`.
- [ ] 6.2 Promo-code field on `web/src/routes/pricing/+page.svelte`, wired to preview then
  checkout, showing the refusal text the API returns.
- [ ] 6.3 Invite page: the account's link with copy-to-clipboard, invitee and paid counts, and
  accrued credit. Follow the existing account-section routing and its auth redirect gate.
- [ ] 6.4 `pnpm --dir web lint` and `pnpm --dir web test` green (a fresh worktree needs
  `svelte-kit sync` first).

## 7. Guards and docs

- [ ] 7.1 A test that walks the module for a string literal shaped like a live promo code and
  fails naming the file. Fixtures use an allowlisted prefix so the guard cannot be satisfied
  by weakening it.
- [ ] 7.2 `internal/identity/promo/AGENTS.md`: the scope, why it is not `engage/referral`, why
  granting is worker-side, and the one-discount-per-session rule.
- [ ] 7.3 Note the new capability in `internal/identity/billing/AGENTS.md` — a `Discount`
  parameter it executes and never interprets — without widening the stated scope.
- [ ] 7.4 Record `INVITE_REWARD_MAX_PER_USER` and `INVITE_REWARD_MAX_PER_RUN` where the other
  worker variables are documented in `CLAUDE.md`.

## 8. Verification

- [ ] 8.1 `gofmt -l .` silent, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`.
- [ ] 8.2 `go test -tags=integration ./internal/identity/promo/ ./internal/identity/billing/`.
- [ ] 8.3 `pnpm check:sql`, `pnpm check:links`, `golangci-lint run`.
- [ ] 8.4 Walk the whole flow against a Stripe stub: invited signup → discounted checkout →
  paid invoice → worker grants → referrer credited; then re-run the worker and assert nothing
  moves.
