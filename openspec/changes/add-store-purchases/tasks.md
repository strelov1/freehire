## 1. Schema

- [x] 1.1 Add `migrations/0135_pro_until_sources.sql`: three nullable `timestamptz` columns
      on `users` (`pro_until_stripe`, `pro_until_revenuecat`, `pro_until_granted`), the
      split of existing values by `stripe_customer_id`, then `DROP COLUMN pro_until` and its
      re-addition as `GENERATED ALWAYS AS (GREATEST(...)) STORED`. Comment the file the way
      `0120_users_pro_until.sql` comments its column: why the split, why generated, and that
      it must run on prod before the deploy.
- [x] 1.2 Answer `pnpm check:sql` on the new file. Every accepted finding carries
      `-- squawk-ignore <rule>` with its reason on the line above; anything without a reason
      gets the statement reshaped instead. `ban-drop-column` and `adding-field-with-default`
      are the two; both reasons are in the file.
- [x] 1.3 Write the reverse migration alongside the forward one, in `deploy/rollback/` and
      deliberately NOT in `migrations/` (initdb runs everything there). It restores
      `pro_until` as a plain column seeded from `GREATEST` and **keeps the three source
      columns** — dropping them would destroy which origin conferred each plan, and a
      re-application would then re-split by `stripe_customer_id` and move money both ways.
      Rolling forward again is its own file, `…reapply.sql`. Both are executed from disk by
      the tests, round trip included.
- [x] 1.4 Replace the `SetProUntil` query with per-source writers. Move the Stripe
      reconciler's near-expiry predicate onto `pro_until_stripe`: on the derived column a
      subscriber whose store subscription or manual grant reaches further would never be
      re-checked. Re-run sqlc; the build is the check that no query still assigns
      `pro_until`. (The RevenueCat near-expiry read moved to 5.1, where it gains a caller.)
- [x] 1.5 Verify on a fresh initdb volume that a clean install produces the generated column
      — testcontainers replays every migration, so the integration tests are that check —
      and that each pre-0135 shape lands in exactly one source column with `pro_until`
      unchanged, by rolling back to that shape and running the real forward file over it.
      Still to do against a copy of prod, with the candidate list from the migration header
      (`pro_until IS NOT NULL AND stripe_customer_id IS NOT NULL`) in hand: those are the
      accounts no query can classify, because a hand-set expiry on top of a real
      subscription looks exactly like one Stripe set.

## 2. The provider seam

- [x] 2.1 Extract the provider interface inside `internal/identity/billing`. It came out
      wider than this task assumed, and the design records why: account ADDRESSING is what
      differs most between the two, so the interface is name, enabled, signatureHeader,
      accept, account, bind, reach, store, dueSoon. Checkout, portal, prices and receipts are
      deliberately outside it — they have no store counterpart. The package split into an
      unexported `engine` (true of both) and `Service` (Stripe, embedding it). No new package,
      so `internal/platform/arch/layering/blocks.go` is untouched.
- [x] 2.2 Move the existing Stripe implementation behind the seam. `verifySignature` moved
      unchanged; the header name became per-provider. Behaviour is identical and the existing
      Stripe tests are the proof — three lines of test changed and none of them an assertion:
      a helper that moved types, a header const written out locally, and `newService` switched
      from New-then-replace-the-client-field to `NewWithBase`, because the provider now takes
      the client at construction and the old aliasing left the stub uncalled.
- [x] 2.3 Point the Stripe path at `pro_until_stripe`. Assert in a test that a Stripe sync
      reporting no subscription leaves `pro_until_revenuecat` and `pro_until_granted`
      untouched. Done ahead of 2.1/2.2 because 0135 makes the old write fail, so the suite
      cannot be green without it; the seam then generalises what is already correct.

## 3. The RevenueCat provider

- [x] 3.1 Configuration from the environment: `REVENUECAT_API_KEY` (secret `sk_`),
      `REVENUECAT_WEBHOOK_SECRET`, `REVENUECAT_ENTITLEMENT` (default `pro`). Absent
      credentials disable this provider alone and leave Stripe working.
- [x] 3.2 Client for `GET /v1/subscribers/{app_user_id}` with the secret key, through the
      SSRF-guarded transport the Stripe client already uses, with the same timeout and error
      body limit.
- [x] 3.3 Derive the entitlement's reach: the configured entitlement id, the later of
      `expires_date` and `grace_period_expires_date`. Table-driven tests for active, in
      grace, cancelled-but-paid, expired, entitlement absent, a different entitlement only,
      an unreadable expiry (entitles nobody), and a null `expires_date` (non-expiring, not
      expired).
- [x] 3.4 Signature verification against RevenueCat's `X-RevenueCat-Webhook-Signature`.
      Tests: unsigned, tampered body, stale timestamp, and a delivery verified over raw
      bytes that a parse-and-reserialise would have rejected.
- [x] 3.5 Parse the delivery envelope for the fields recording needs — `id`, `app_user_id`,
      `type` — and nothing else. The event's own claims about entitlement are deliberately
      not read.
- [x] 3.6 Enforce that a subscriber read happens only for an account with a recorded
      RevenueCat event or a non-NULL `pro_until_revenuecat`, with a test that the reconciler
      makes no request for an account with neither. **Revised after review:** the check binds
      the BULK passes (`SyncUser`), not the read itself. Inside the read it also bound the
      sync route, and a first-time buyer has neither footprint by definition — so a purchase
      whose first webhook was lost could never be recovered by any path. `SyncCaller` is the
      self-service entry, and the spec now states the distinction.

## 4. HTTP surface

- [x] 4.1 `POST /api/v1/billing/revenuecat/webhook`: verify over the raw body, record,
      answer 200, then apply. Mounted only when the provider is configured.
- [x] 4.2 Idempotency test across providers: the same event id from Stripe and from
      RevenueCat both record and both apply; the same id twice from one provider records
      once and answers success both times.
- [x] 4.3 `POST /api/v1/billing/revenuecat/sync`: cookie-authenticated, identifies the
      caller from the session, ignores any user identifier in the request, rate-limited per
      caller. Tests for the anonymous call, the call carrying somebody else's id, and the
      call past the allowance.
- [x] 4.4 Add `pro_source` to `GET /me/plan`: the source column equal to the derived value,
      tie broken in the order `stripe`, `revenuecat`, `granted`, absent on the free plan.
      Test the tie and the free case explicitly.
- [x] 4.5 Confirm the plan surface still answers with the provider unreachable — it reads
      columns and must make no provider call.

## 5. The reconciler

- [x] 5.1 Extend `cmd/billing-sync` to a pass per configured provider, each writing only its
      own column, each reporting its own outcome. Add the RevenueCat near-expiry read here
      rather than in group 1, so the query arrives with the caller that needs it — its
      predicate is on `pro_until_revenuecat`, for the reason 1.4 states about the derived
      column.
- [x] 5.2 One provider unreachable must not stall the other: the failing pass is reported
      and retried, the other completes.
- [x] 5.3 Integration test of the lost-delivery path: a purchase whose webhook never
      arrived becomes Pro on the next pass.

## 6. Ops

- [x] 6.1 Record the manual dashboard steps in `deploy/`: the RevenueCat project, the `pro`
      entitlement, the offering, the App Store and Play product mappings, and registering the
      webhook with HMAC signing enabled. In `deploy/AGENTS.md`, beside Stripe's equivalent.
- [ ] 6.2 NEEDS HOST ACCESS — add the three variables to `/opt/freehire/.env` on the host.
- [x] 6.3 Update the support runbook: granting Pro by hand is now
      `UPDATE users SET pro_until_granted = …`, and the old statement fails on purpose
      (428C9). In `deploy/AGENTS.md`, with the reason the other two columns are off limits.
- [ ] 6.4 NEEDS HOST ACCESS — run the migration manually on prod, before deploying the code that reads the new
      columns.

## 7. Verification

- [ ] 7.1 NEEDS DASHBOARD ACCESS — enable HMAC signing on the webhook integration and keep the
      signing secret as `REVENUECAT_WEBHOOK_SECRET`. **Decided: there is no fallback.** If the
      integration offers only the shared `Authorization` header, stop — that is a bearer
      credential anyone who sees one delivery can replay, no code path accepts it, and the
      right response is to reconsider the integration rather than weaken the check.
- [ ] 7.2 NEEDS DASHBOARD ACCESS — determine whether RevenueCat re-signs each webhook retry.
      **How:** cause one delivery to fail (point the endpoint at a URL that answers 500, or
      pause the service), wait for a retry, then compare the `t=` value in the retry's
      `X-RevenueCat-Webhook-Signature` against the original's in the dashboard's delivery log.
      A NEW `t` means each retry is re-signed and `revenuecatSignatureWindow` can drop to five
      minutes, matching Stripe. The SAME `t` means it must stay wide enough for the 80-minute
      tail, which is what it is now. Until this is answered the wide window stands: it is the
      direction where being wrong costs a wasted provider call rather than a paid subscription.
- [ ] 7.3 NEEDS STORE ACCESS — sandbox purchase end to end on both stores: purchase confers Pro, cancellation
      leaves the paid period standing, expiry lapses without a sweep, refund revokes.
- [ ] 7.4 NEEDS STAGING ACCESS — cross-provider check on a staging account: a Stripe subscription and a store
      subscription on one account yield the longer reach, and cancelling either leaves the
      other standing.
- [x] 7.5 Confirm `pro_source` reports what the mobile client needs before that change
      starts — it is the contract `freehire-mobile` builds against. Pinned by
      `TestPlanNamesWhereProCameFrom`: the store case, the furthest-source case, the stated
      tie-break, a free account and a lapsed one.
