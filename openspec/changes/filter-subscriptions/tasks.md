## 1. Schema & data access

- [x] 1.1 Add migration `0022_filter_subscriptions.sql` creating `subscriptions`, `subscription_matches` (PK `(subscription_id, job_id)` + `claimed_at` lease + partial pending index), and `telegram_links` per the design
- [x] 1.2 Add hand-written sqlc queries: create/list/toggle/delete subscriptions (owner-scoped), list active subscriptions joined to saved-search query, record match (INSERT ... ON CONFLICT DO NOTHING), claim pending matches with `FOR UPDATE SKIP LOCKED` + lease, mark matches notified, bump attempts / dead-letter, release claim, upsert/get/delete telegram_links
- [x] 1.3 Run `make sqlc`, commit generated `internal/db` code; `go build ./...` + integration tests green (`go test -tags=integration ./internal/db/`)

## 2. Shared filter builder refactor

- [x] 2.1 Extract the Meili filter builder into `internal/search.FilterFromValues(url.Values)` (pure) + move the facet vocabulary to `search.StringFacets`; unit-tested
- [x] 2.2 Rewire the search + facets handlers to the shared function/vocabulary; build, vet, handler + search unit tests green

## 3. Matching + delivery engine (`internal/notify`)

- [x] 3.1 Defined the `Notifier` interface (`Send(ctx, channel, dest, Digest) error`) + `Digest`/`DigestJob` value types; `Searcher`/`Store` ports; `Config`/`Stats`/`Runner`
- [x] 3.2 Implemented MATCH: group active subscriptions by query, run each distinct query (sort `created_at:desc`, bounded limit, keyword-only), record matches gated by per-subscription `start_at`; unit-tested dedup (shared-query → one search), the `start_at` gate, idempotent re-scan via fake searcher/store
- [x] 3.3 Implemented DELIVER: skip-locked lease claim, group per subscription, one digest, mark notified on success / record-failure + dead-letter on error; unit-tested one-digest-per-subscription and failure-stays-pending
- [x] 3.4 Resolve the telegram recipient from `telegram_links`; soft-skip + release-claim (no attempt counted) when unlinked; unit-tested the skip path

## 4. Telegram channel (`telegram-notify`)

- [x] 4.1 Implemented the signed deep-link token (`LinkTokens` mint+verify, short TTL, `purpose=tg-link`) reusing `JWT_SECRET`; unit-tested round-trip/expiry/forgery/wrong-purpose
- [x] 4.2 Implemented the Bot API `Client` (sendMessage/setWebhook over net/http), webhook `Update` parsing + `StartToken`, and `Notifier` (HTML digest render + send); unit-tested render/escaping, request shaping, API-error propagation, `/start` parsing
- [x] 4.3 Added bot config (`TELEGRAM_BOT_TOKEN`/`_BOT_USERNAME`/`_WEBHOOK_SECRET`) to `internal/config`; feature disabled when token unset

## 5. `cmd/notify` worker

- [ ] 5.1 Wire `cmd/notify/main.go` via `worker.Bootstrap`: run one MATCH→DELIVER pass and exit; pick the channel `Notifier` from config
- [ ] 5.2 Add the `notify` binary to the Dockerfile build + COPY list

## 6. HTTP surface

- [ ] 6.1 Telegram linking handlers (`RequireAuth`): `POST /api/v1/me/telegram/link` (returns deep link), `GET /api/v1/me/telegram` (status), `DELETE /api/v1/me/telegram` (unlink); integration-test
- [ ] 6.2 Webhook handler `POST /api/v1/telegram/webhook` (unauthenticated): verify secret-token header, parse `/start <token>`, store `chat_id`, confirm to the user; integration-test secret rejection + happy path
- [ ] 6.3 Subscription handlers (`RequireAuth`): list/create/toggle/delete under `/api/v1/me/subscriptions`, owner-scoped, duplicate + cross-user guards; integration-test
- [ ] 6.4 Expose the bot username / feature-enabled flag in public config for the SPA

## 7. SPA

- [ ] 7.1 Add a "Notify on Telegram" toggle per saved search, gated on link status, with a deep-link dialog for first-time linking; verify via svelte-check + lint

## 8. Verification & ops

- [ ] 8.1 `go build ./... && go vet ./... && go test ./...` green; recompile integration tests after handler/constructor changes
- [ ] 8.2 Document deploy steps in the change (set env, `setWebhook` with secret, add `notify` cron with flock, no reindex)
