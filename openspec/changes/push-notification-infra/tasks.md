## 1. Schema

- [x] 1.1 `migrations/0085_user_push_tokens.sql`: `user_push_tokens(id, user_id FK CASCADE, token UNIQUE, platform CHECK ios|android, last_seen_at, created_at)`
- [x] 1.2 `internal/db/queries/push_tokens.sql`: `UpsertPushToken` (ON CONFLICT (token) DO UPDATE user_id/last_seen_at), `DeletePushToken` (by token + owning user_id), `ListPushTokensForUser`, `PruneDeadPushToken` (for the dead-token prune path, no user_id check needed there)
- [x] 1.3 `make sqlc` — regenerate `internal/db`

## 2. Notifier package

- [x] 2.1 `internal/pushnotify/pushnotify.go`: `Notifier` interface (`Send(ctx, token, title, body string) error`), `ExpoNotifier` implementation posting to `https://exp.host/--/api/v2/push/send`
- [x] 2.2 Parse Expo's per-message send ticket; on `DeviceNotRegistered` prune the token immediately and return nil; other error statuses return an error, token untouched
- [x] 2.3 `migrations/0086_push_ticket_outbox.sql`: `push_ticket_outbox(id, token, ticket_id, created_at)` — no FK to users (a token may already be gone by check time; prune is idempotent by token value)
- [x] 2.4 `internal/db/queries/push_ticket_outbox.sql`: `EnqueuePushTicket`, `ClaimDuePushTickets` (older than a minimum age, batched), `DeletePushTickets` (by id list); `make sqlc`
- [x] 2.5 `ExpoNotifier`: on a successful (`ok`) send ticket, enqueue it via a `TicketQueuer` seam instead of just returning nil; add `CheckReceipts(ctx) error` — claims a due batch, `POST /getReceipts`, prunes any `DeviceNotRegistered` result, deletes all processed rows from the outbox regardless of outcome

## 3. Handlers

- [x] 3.1 `internal/handler/me_push_tokens.go`: `RegisterPushToken` (`POST /me/push-tokens`), `UnregisterPushToken` (`DELETE /me/push-tokens`), `TestPushToken` (`POST /me/push-tokens/test`) — all under the existing cookie-auth `/me` group
- [x] 3.2 Wire the three routes into the route table alongside the other `/me/*` registrations

## 4. Verification

- [x] 4.1 `go vet -tags=integration ./...` passes
- [x] 4.2 End-to-end proof: `TestPushTokensEndToEnd` (`internal/handler`, `-tags=integration`) drives the real HTTP handlers against a real Postgres (testcontainers) — register/reassign/unregister/self-test, all over actual requests, not mocks. A literal manual `curl` against a locally running server was attempted but skipped: the default local Postgres port was already held by another active dev session, and touching it was the wrong call mid-session. The integration test is the stronger proof of the two (asserts on actual DB state after each HTTP call, which a manual curl session would only eyeball) — a real Expo round trip is unverified by either, since a live push requires a real device-issued token this environment does not have.

## 5. Receipt-polling worker

- [x] 5.1 `cmd/push-receipts/main.go`: run-once-and-exit worker (`worker.Bootstrap`), needs only `DATABASE_URL`; calls `ExpoNotifier.CheckReceipts` once per run
- [x] 5.2 Document the cron schedule expectation (every 15-20 min) in the command's doc comment, matching `cmd/remind`'s style

## 6. Device listing (added while building the mobile client)

The first real consumer showed a gap: with register/unregister/test but no read,
the app cannot answer "is this device registered?", and the OS permission cannot
answer it either (an app cannot revoke its own notification permission, so
permission stays granted after a user switches push off). The client's only
alternative was a locally persisted opt-in flag — a second source of truth that
drifts from the backend the moment a token is reassigned or pruned by
`cmd/push-receipts`. Closing the gap here is the smaller, truer fix.

- [x] 6.1 `ListPushTokens` (`GET /me/push-tokens`) in `internal/handler/me_push_tokens.go`: the caller's own devices as `{"data":[{token,platform,created_at,last_seen_at}],"meta":{"total":N}}`, cookie-only and owner-scoped like its siblings. Reuses the existing `ListPushTokensForUser` query — no new SQL, no `make sqlc`.
- [x] 6.2 Route wired in `internal/handler/auth.go` alongside the other `/me/push-tokens` registrations.
- [x] 6.3 Tests: the cookie-only table in `me_push_tokens_test.go` gains `GET` (no-credential and bearer-only); `TestPushTokensEndToEnd` gains a `list is owner-scoped` subtest asserting that after reassignment the token shows for bob and not for alice.
- [ ] 6.4 **Verification pending** — `go build/vet/test` were NOT run for 6.1–6.3: the environment that wrote them has no Go module cache and no access to `proxy.golang.org`, so the dependencies cannot be fetched. Run `go build ./... && go vet ./... && go test ./... && go vet -tags=integration ./...` before merging.
