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

- [x] 5.1 Wired `cmd/notify/main.go` via `worker.Bootstrap`: one MATCH→DELIVER pass, telegram Notifier from config, feature-disabled (exit 0) when search/bot unconfigured
- [x] 5.2 Added the `notify` binary to the Dockerfile build + COPY list

## 6. HTTP surface

- [x] 6.1 Telegram linking handlers (`RequireAuth`): `POST /me/telegram/link` (deep link), `GET /me/telegram` (status + `enabled`), `DELETE /me/telegram` (unlink); integration-tested
- [x] 6.2 Webhook handler `POST /telegram/webhook` (unauthenticated): secret-token header verified, `/start <token>` parsed, `chat_id` stored, bot confirms; integration-tested secret rejection + happy path + bogus token
- [x] 6.3 Subscription service + handlers (`RequireAuth`): list/create/toggle/delete under `/me/subscriptions`, owner-scoped, duplicate/invalid-channel/cross-user guards; integration-tested
- [x] 6.4 Exposed the feature-`enabled` flag in the authed `GET /me/telegram` status (toggle lives in the authed area; deep-link URL is built server-side, so no public username/config endpoint needed)

## 7. SPA

- [x] 7.1 Added a "🔔 Notify on Telegram" toggle to the saved-search picker (shown when the current filters match a saved set), with connect-then-recheck deep-link flow when unlinked; api.ts methods + notifications store; svelte-check clean (0 errors)
- [x] 7.2 Discoverability: `/my/notifications` management page (connect/disconnect Telegram + pause/resume/remove subscriptions) linked from the user menu; a CTA hint in the filters panel when no saved set is active; a homepage section. setSubscriptionActive api+store method added. svelte-check clean

## 8. Verification & ops

- [x] 8.1 `go build` / `go vet` / `go test ./...` green; gofmt clean across the diff; new db + handler integration tests green; svelte-check clean
- [x] 8.2 Documented deploy steps in design.md Migration Plan (bot @free_hire_bot, env vars, `setWebhook` curl, flock cron, manual 0022, no reindex, token rotation)
