## 1. Schema

- [x] 1.1 `migrations/0085_user_push_tokens.sql`: `user_push_tokens(id, user_id FK CASCADE, token UNIQUE, platform CHECK ios|android, last_seen_at, created_at)`
- [x] 1.2 `internal/db/queries/push_tokens.sql`: `UpsertPushToken` (ON CONFLICT (token) DO UPDATE user_id/last_seen_at), `DeletePushToken` (by token + owning user_id), `ListPushTokensForUser`, `PruneDeadPushToken` (for the dead-token prune path, no user_id check needed there)
- [x] 1.3 `make sqlc` — regenerate `internal/db`

## 2. Notifier package

- [x] 2.1 `internal/pushnotify/pushnotify.go`: `Notifier` interface (`Send(ctx, token, title, body string) error`), `ExpoNotifier` implementation posting to `https://exp.host/--/api/v2/push/send`
- [x] 2.2 Parse Expo's per-message send ticket; on `DeviceNotRegistered` prune the token immediately and return nil; other error statuses return an error, token untouched
- [ ] 2.3 `migrations/0086_push_ticket_outbox.sql`: `push_ticket_outbox(id, token, ticket_id, created_at)` — no FK to users (a token may already be gone by check time; prune is idempotent by token value)
- [ ] 2.4 `internal/db/queries/push_ticket_outbox.sql`: `EnqueuePushTicket`, `ClaimDuePushTickets` (older than a minimum age, batched), `DeletePushTickets` (by id list); `make sqlc`
- [ ] 2.5 `ExpoNotifier`: on a successful (`ok`) send ticket, enqueue it via a `TicketQueuer` seam instead of just returning nil; add `CheckReceipts(ctx) error` — claims a due batch, `POST /getReceipts`, prunes any `DeviceNotRegistered` result, deletes all processed rows from the outbox regardless of outcome

## 3. Handlers

- [ ] 3.1 `internal/handler/me_push_tokens.go`: `RegisterPushToken` (`POST /me/push-tokens`), `UnregisterPushToken` (`DELETE /me/push-tokens`), `TestPushToken` (`POST /me/push-tokens/test`) — all under the existing cookie-auth `/me` group
- [ ] 3.2 Wire the three routes into the route table alongside the other `/me/*` registrations

## 4. Verification

- [ ] 4.1 `go vet -tags=integration ./...` passes
- [ ] 4.2 Manual: register a token via curl with a real session cookie, call the test-send endpoint, confirm a `200` and an Expo API call in logs (a fake/unassigned Expo token will get a documented "not a registered push notification recipient" response from Expo itself — acceptable, proves the round trip)

## 5. Receipt-polling worker

- [ ] 5.1 `cmd/push-receipts/main.go`: run-once-and-exit worker (`worker.Bootstrap`), needs only `DATABASE_URL`; calls `ExpoNotifier.CheckReceipts` once per run
- [ ] 5.2 Document the cron schedule expectation (every 15-20 min) in the command's doc comment, matching `cmd/remind`'s style
