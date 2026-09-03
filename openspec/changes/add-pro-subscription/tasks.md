## 0. Read the provider contract before writing a client — DONE 2026-09-03

Verdict **VALIDATED**: the hosted paywall accepts our identifier, so the checkout decision
stands. Four corrections came out of it and are carried into the tasks below; the full
findings are in the design's "The provider contract, as verified".

- [x] 0.1 Webhook envelope confirmed — `{api_version, event{id, app_user_id, type,
      expiration_at_ms, entitlement_ids}}`, and **retries reuse the same `id`**
- [x] 0.2 Authentication confirmed — HMAC signing is available and supersedes the shared
      `Authorization` value this change originally specified
- [x] 0.3 Subscriber endpoint confirmed — `GET /v1/subscribers/{id}` with a **secret** key;
      entitlements carry `expires_date` (**nullable**) and `grace_period_expires_date`;
      the response also carries `management_url`. **The GET creates the subscriber if the
      id is unknown**
- [x] 0.4 Checkout confirmed — `https://pay.rev.cat/<token>/<url-encoded app_user_id>`, a
      **path segment**, not a query parameter
- [x] 0.5 Copy the findings into `internal/identity/billing/AGENTS.md` with the date read
- [ ] 0.6 Confirm in the RevenueCat dashboard that HMAC signing is offered on this
      integration. **If it is not**, fall back to the shared `Authorization` value and say
      so in the package's `AGENTS.md` — the freshness window and replay scenario then do
      not apply, and the spec's signature requirement needs amending rather than faking

## 1. Schema

- [x] 1.1 Migration `0128_billing_events.sql`: `billing_events` (id, provider, event_id,
      user_id, event_type, payload jsonb, received_at, processed_at NULL) with
      `UNIQUE (provider, event_id)` and an index serving "unprocessed, oldest first"
- [x] 1.2 `user_id` references `users` with the same on-delete behaviour the rest of the
      user-owned tables use, so account deletion erases these rows by cascade
- [x] 1.3 `pnpm check:sql` over the added file
- [x] 1.4 sqlc queries: insert-event-if-new, list-unprocessed, mark-processed,
      list-subscribers-near-expiry, delete-for-user; then `make sqlc` and commit the
      regenerated output. `SetProUntil` already exists from `add-plan-limits`, and the
      near-expiry query walks `billing_events` rather than `users` — it is cheaper, and it
      makes "we only ask the provider about someone who has transacted" a property of the
      query instead of a rule the worker must remember

## 2. The billing package

- [x] 2.1 Create `internal/identity/billing` and add it to the `identity` list in
      `internal/platform/arch/layering/blocks.go`; confirm `golangci-lint run` and the
      layering test pass with it in the table
- [x] 2.2 Configuration from the environment: `REVENUECAT_API_KEY` (the `sk_` secret key),
      `REVENUECAT_WEBHOOK_SECRET` (HMAC signing secret), `REVENUECAT_ENTITLEMENT` (default
      `pro`), `BILLING_CHECKOUT_URL` (everything up to and including the paywall token).
      An absent API key or webhook secret means billing is disabled; assert by test that
      construction succeeds and reports disabled rather than failing
- [x] 2.3 The provider client: `GET /v1/subscribers/{url-encoded id}` over
      `platform/safehttp` with `Authorization: Bearer sk_…`, a short timeout and no retry
      of its own (the reconciler is the retry). Parse `entitlements`, `management_url`
- [x] 2.4 `proUntilFrom(state) time.Time` as a **pure function** of the provider's parsed
      state: across entitlements conferring Pro, take the later of `expires_date` and
      `grace_period_expires_date`, then the latest across entitlements; zero when there is
      none. Table test with no network and no database, covering active, lapsed, refunded,
      transferred, several entitlements, an unknown entitlement, **an entitlement in its
      grace period**, and **a null `expires_date`, which means never-expiring and must not
      read as expired**
- [x] 2.5 `Sync(ctx, userID)`: read state, derive, write `users.pro_until`. Idempotent —
      a test applies the same state twice and asserts the second write changes nothing.
      **Callers must already hold an event or a non-NULL `pro_until` for the user**, since
      the provider's GET creates a subscriber for an unknown id
- [x] 2.6 `RecordEvent(ctx, event)`: insert-if-new, returning whether it was new. A test
      delivers the same event twice and asserts one row
- [x] 2.7 `verifySignature(raw []byte, header, secret, now)` as a pure function: parse
      `t=<unix>,v1=<hex>`, recompute HMAC-SHA256 over `"<t>.<raw>"`, compare with
      `crypto/subtle`, and reject a `t` outside the freshness window. Tests: valid, wrong
      secret, absent header, malformed header, **stale timestamp**, and **a body that
      round-tripped through a JSON parse must still verify from the raw bytes**
- [x] 2.8 `AGENTS.md` for the package: what it is, that it is freehire.me's hosted
      billing and unsupported for self-hosting, that it is inert without configuration,
      and the ownership rule for `users.pro_until`

## 3. HTTP surface

- [x] 3.1 `POST /api/v1/billing/revenuecat/webhook`: verify the signature over `c.Body()`
      (the raw bytes — never a re-marshalled struct), record, then attempt to apply inline
      with a short timeout. The apply runs BEFORE the response is flushed — a handler
      cannot write and then keep working without an unmanaged goroutine, and 10s inside a
      60s budget does not need one. The guarantee is not the ordering but the outcome: a
      failure to apply is logged and does **not** change the 200, while a failure to
      *record* returns non-200 so the provider retries
- [x] 3.2 The whole handler must finish well inside the provider's **60-second** budget;
      bound the inline apply accordingly
- [x] 3.3 The route is unauthenticated by session and authenticated by the signature only;
      confirm it is exempt from the auth middleware and included in the rate limiter
- [x] 3.4 `GET /api/v1/billing/checkout` behind `RequireAuth`: returns
      `BILLING_CHECKOUT_URL` + `/` + the URL-**path**-encoded `users.id` of the caller. A
      client-supplied identifier is ignored
- [x] 3.5 With billing disabled, both routes return 404. Integration test asserts this and
      asserts the rest of the API is unaffected
- [x] 3.6 Integration tests: duplicate delivery, bad signature, stale signature, unknown
      event type, event for an unknown user, provider unreachable during the inline apply

## 4. The reconciler

- [x] 4.1 `cmd/billing-sync` on `platform/worker`'s Main/Bootstrap, `Type=oneshot`
      semantics, exits non-zero on failure
- [x] 4.2 Without billing configuration it exits 0 **without opening the pool** — the
      `cmd/queue-metrics` shape; assert by test
- [x] 4.3 Pass one: apply unprocessed `billing_events`, oldest first, marking each
      processed. One failing user does not abort the run
- [x] 4.4 Pass two: re-sync users whose `pro_until` falls in a window around now
- [x] 4.5 Bounded per run (`BILLING_SYNC_MAX_PER_RUN`), so a backlog cannot turn a oneshot
      unit into a run that outlives its timer
- [x] 4.6 Integration test: a renewal that produced no webhook is corrected by a run

## 5. SPA

- [x] 5.1 An upgrade entry point on the usage surface and from the 402 body's pointer,
      calling `GET /api/v1/billing/checkout` and navigating to the returned URL
- [x] 5.2 "You are on Pro until <date>" on the usage surface when `pro_until` is in force
- [x] 5.3 With billing disabled the upgrade entry point is absent, not broken — the 404
      must never reach a user as an error
- [x] 5.4 The delete-account surface states that deletion does not cancel the subscription
      and links to the `management_url` the provider reports for that subscriber — not to
      a URL we composed
- [x] 5.5 eslint over the touched files (clean). `pnpm check:dead` flags nothing of ours; its other findings are pre-existing and locally unreliable — the worktree has no `extension/` or `design-system/` dependencies installed, which is why CLAUDE.md calls that check CI-only

## 6. Ops

- [x] 6.1 `deploy/`: the `freehire-billing-sync` service and timer (hourly), and the entry
      in `deploy/AGENTS.md`. **Copy them to the host, and build the binary there** —
      `release.sh` builds the API, not every command in `cmd/`. Hourly is
      chosen against the provider's retry ceiling: it gives up after five attempts over
      about two and a half hours, and after that this timer is the only path left
- [x] 6.2 Record the manual dashboard steps (register the webhook URL, enable HMAC signing
      and capture the secret, mint the `sk_` key, create the `pro` entitlement and the Web
      Billing paywall, capture the paywall token) where the other manual host steps live
- [ ] 6.3 Add the four variables to `/opt/freehire/.env` on the host. Confirm which env
      file the worker unit actually reads before assuming — a variable in the wrong file
      degrades silently
- [x] 6.4 Confirm `.gitleaks.toml` needs no new allowlist entry, and that no key reached a
      tracked file
- [x] 6.5 Do **not** add billing to `docker-compose`'s default services

## 7. Verification

- [x] 7.1 `gofmt -l .` prints nothing; `go vet ./...`; `go test ./...`
- [x] 7.2 `go vet -tags=integration ./...` before pushing
- [x] 7.3 `go test -tags=integration ./internal/identity/billing/ ./internal/api/handler/`
- [x] 7.4 `pnpm check:links` — this design links to files by path
- [ ] 7.5 End-to-end on prod with a real purchase of the live product, then a refund:
      assert `pro_until` is set, then cleared, and that `billing_events` records both
