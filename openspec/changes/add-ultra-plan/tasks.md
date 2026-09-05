## 1. Schema

- [ ] 1.1 Write `migrations/0141_ultra_until_sources.sql`: `ultra_until_stripe`,
  `ultra_until_revenuecat`, `ultra_until_granted`, and a derived `ultra_until` that refuses
  assignment — migration 0135's shape, applied again and saying why in its own comment.
  Verify with `pnpm check:sql`.
- [ ] 1.2 Add the queries to `internal/platform/db/queries/plan.sql`: one setter per source
  column, the near-expiry sweep per provider, and a read that returns both `pro_until` and
  `ultra_until` in one row. Run `make sqlc` and commit the generated diff.

## 2. The tier model

- [ ] 2.1 `plan.TierUltra`, and `TierOf(proUntil, ultraUntil, now)` resolving Ultra > Pro >
  Free. Table-test the boundaries, including both-live and just-lapsed.
- [ ] 2.2 Reshape `featureConfig` to carry an `Allowance` per tier with fair use attached to
  the unlimited ones. `Allowance(tier, feature)` becomes a lookup; assert an unconfigured
  feature still allows nothing.
- [ ] 2.3 Generalise the fair-use branch in `decide.go` from `ProFairUse(f)` to
  `FairUse(tier, f)`, and cover an ultra account hitting its guard.
- [ ] 2.4 `Store.Tier` reads both columns in one query — not two, so a tier cannot be
  resolved from two different instants.
- [ ] 2.5 `session.go`'s tier branch learns the third tier.
- [ ] 2.6 Environment: `PLAN_PRO_DAILY_<F>` and `PLAN_ULTRA_DAILY_<F>` beside the existing
  knobs; an unparseable value logs and keeps the default, as the others do.
- [ ] 2.7 The numbers: auto-apply 0/3/unlimited; tailor 2/40/120; match 3/60/180; assistant
  10/200/600; dictation 10/200/600; cover-letter 3/60/180.

## 3. Billing: a value per tier

- [ ] 3.1 Introduce `billing.entitlement{Pro, Ultra time.Time}` and widen `reach` and `store`
  to carry it. A zero `entitlement` must write both columns as NULL.
- [ ] 3.2 `entitlement.go`: resolve each tier from its own price list, reusing the existing
  filter rather than a second copy of it.
- [ ] 3.3 `config.go`: read `STRIPE_ULTRA_PRICE_IDS`. An unset list must leave pro and free
  behaving byte-identically to today — assert it against the stub.
- [ ] 3.4 `stripeProvider`: report and write both tiers from one re-read of the subscriber.
- [ ] 3.5 `revenuecatProvider`: report zero for ultra and write it anyway. Assert that a
  store sync cannot clear a web Ultra entitlement, and vice versa.
- [ ] 3.6 `dueSoon` reaches accounts near expiry on EITHER entitlement, or a lapsing Ultra is
  never reconciled.

## 4. auto-apply as a metered feature

- [ ] 4.1 `plan.FeatureAutoApply`, shipping with `enforce: true` — the one feature that does.
  Assert it refuses while the shadow-mode features still pass.
- [ ] 4.2 `PostJobAutoApply`: replace the `!= TierPro` check with `Consume`, keyed by the
  posting's id, placed after the platform / CV / already-applied checks and before the queue
  write.
- [ ] 4.3 Assert the spend rules: a second request for the same posting spends nothing; a
  request refused for no CV spends nothing; a free account is refused; an ultra account is
  never refused.
- [ ] 4.4 Assert the refusal names auto-apply and carries the day's figures.

## 5. Web

- [ ] 5.1 `PlansMatrix` in `web/src/lib/types.ts` carries a third number per feature, and the
  plans endpoint returns it.
- [ ] 5.2 `/pricing` renders three plans. Nothing about a tier is hard-coded in the page —
  the numbers stay the API's.
- [ ] 5.3 The upgrade button picks the right price per plan, and a deployment offering no
  Ultra price shows two plans rather than a dead third column.
- [ ] 5.4 `pnpm --dir web lint` and `pnpm --dir web test` green (a fresh worktree needs
  `svelte-kit sync` first).

## 6. Docs

- [ ] 6.1 `internal/ai/plan/AGENTS.md`: three tiers, the per-tier allowance shape, and why
  auto-apply is the one feature that enforces on arrival.
- [ ] 6.2 `internal/identity/billing/AGENTS.md`: a provider answers for every tier it can
  sell, and writes every column it owns on every sync.
- [ ] 6.3 Record `STRIPE_ULTRA_PRICE_IDS`, `PLAN_PRO_DAILY_*` and `PLAN_ULTRA_DAILY_*` where
  the other environment variables are documented in `CLAUDE.md`.

## 7. Verification

- [ ] 7.1 `gofmt -l .` silent, `go vet ./...`, `go test ./...`,
  `go vet -tags=integration ./...`.
- [ ] 7.2 `go test -tags=integration ./internal/ai/plan/ ./internal/identity/billing/
  ./internal/api/handler/`.
- [ ] 7.3 `pnpm check:sql`, `pnpm check:links`, `golangci-lint run --new-from-rev=origin/main`.
- [ ] 7.4 Walk it against a Stripe stub: an Ultra price resolves to ultra, auto-apply is
  unbounded there and refuses on pro at the fourth attempt in a day, and a store sync leaves
  the web entitlement alone.
