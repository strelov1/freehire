## 1. Schema

- [x] 1.1 `migrations/0085_user_push_tokens.sql`: `user_push_tokens(id, user_id FK CASCADE, token UNIQUE, platform CHECK ios|android, last_seen_at, created_at)`
- [x] 1.2 `internal/db/queries/push_tokens.sql`: `UpsertPushToken` (ON CONFLICT (token) DO UPDATE user_id/last_seen_at), `DeletePushToken` (by token + owning user_id), `ListPushTokensForUser`, `DeletePushTokenByValue` (for the dead-token prune path, no user_id check needed there)
- [x] 1.3 `make sqlc` — regenerate `internal/db`

## 2. Notifier package

- [x] 2.1 `internal/pushnotify/pushnotify.go`: `Notifier` interface (`Send(ctx, token, title, body string) error`), `ExpoNotifier` implementation posting to `https://exp.host/--/api/v2/push/send`
- [x] 2.2 Parse Expo's per-message receipt; on `DeviceNotRegistered` delete the token row (via the store) and return nil (pruned, not a caller-facing error); other error statuses return an error, token untouched

## 3. Handlers

- [ ] 3.1 `internal/handler/me_push_tokens.go`: `RegisterPushToken` (`POST /me/push-tokens`), `UnregisterPushToken` (`DELETE /me/push-tokens`), `TestPushToken` (`POST /me/push-tokens/test`) — all under the existing cookie-auth `/me` group
- [ ] 3.2 Wire the three routes into the route table alongside the other `/me/*` registrations

## 4. Verification

- [ ] 4.1 `go vet -tags=integration ./...` passes
- [ ] 4.2 Manual: register a token via curl with a real session cookie, call the test-send endpoint, confirm a `200` and an Expo API call in logs (a fake/unassigned Expo token will get a documented "not a registered push notification recipient" response from Expo itself — acceptable, proves the round trip)
