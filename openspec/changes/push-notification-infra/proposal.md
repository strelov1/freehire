## Why

freehire's mobile app has no way to reach a user who isn't actively looking at
it — every existing notification channel (email, Telegram) assumes the user
checks an inbox or has linked a bot. A mobile push channel is the natural fit
for the app's own lifecycle nudges and reminders, but building it requires
device-token storage and a send path that don't exist yet. This change lays
that groundwork — token registration and a working send capability — without
wiring push into any of the three existing notification engines
(`internal/notify`/`internal/reminder`/`internal/nudge`) yet; that's
deliberately deferred to a follow-up change once the infrastructure is proven
live.

## What Changes

- New `user_push_tokens` table: one row per registered device (a user may have
  several), keyed by the push token itself.
- `POST /api/v1/me/push-tokens` — the mobile app registers (or refreshes) its
  Expo push token after sign-in and after the OS grants notification
  permission. Upserts by token, so a token that changes owner (a different
  account signs in on the same device) reassigns cleanly instead of
  duplicating.
- `DELETE /api/v1/me/push-tokens` — unregisters the caller's token (sign-out,
  account deletion).
- `GET /api/v1/me/push-tokens` — the caller's own registered devices. Added
  once the mobile client was built: without a read, an app cannot tell whether
  the device it is running on is registered, and the OS permission cannot tell
  it either (permission stays granted after a user turns push off in-app). The
  alternative was a locally persisted flag on the client that drifts from the
  backend whenever a token is reassigned or pruned.
- New `internal/pushnotify` package: a `Notifier` that sends one push message
  through the Expo Push API (`https://exp.host/--/api/v2/push/send`). No
  APNs/FCM credentials — Expo's relay handles both platforms from one token
  format, keyed to the app's own Expo project.
- Expo's send response is a ticket, not a final delivery outcome — a token
  that just went dead (fresh uninstall/permission revoke) is only
  discoverable later via Expo's `getReceipts` endpoint. New
  `push_ticket_outbox` table queues sent ticket ids; `cmd/push-receipts`
  (a new cron worker) polls them after Expo's recommended wait and prunes
  any token whose receipt comes back `DeviceNotRegistered`.
- `POST /api/v1/me/push-tokens/test` — sends one push to the caller's own
  registered token(s), so the infrastructure is verifiable end-to-end (device
  → token → backend → Expo → device) without needing a real notification
  event to trigger it.

## Capabilities

### New Capabilities

- `push-notify`: device push-token registration and the Expo Push API send
  path. Mirrors the shape of the existing `email-notify`/`telegram-notify`
  capabilities (one channel, its own send mechanics) but stops short of
  digest rendering or engine wiring — this change is infrastructure only.

### Modified Capabilities

(none — no existing requirement's behavior changes)

## Impact

- `migrations/0085_user_push_tokens.sql`, `migrations/0086_push_ticket_outbox.sql`
  (new tables), `internal/db/queries/` + `make sqlc` (new queries).
- `cmd/push-receipts`: new run-once-and-exit cron worker, needs `DATABASE_URL`
  only (Expo's API needs no credentials).
- `internal/pushnotify/`: new package, `Notifier` interface + Expo Push API
  client.
- `internal/handler/me_push_tokens.go`: new handler (register/unregister/test
  routes), wired under the existing `/me` route group.
- No changes to `internal/notify`, `internal/reminder`, or `internal/nudge` —
  push is not yet a delivery channel for any of them.
- Mobile app (`freehire-mobile`): out of scope for this change. Registering a
  real Expo push token requires `expo-notifications` and permission UI, which
  don't exist there yet — a separate, later change.
