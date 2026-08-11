## Context

freehire has three independent notification engines
(`internal/notify`/`internal/reminder`/`internal/nudge`), each with its own
`Notifier`/`Router` pair over a shared two-channel vocabulary (email,
Telegram) — see `docs/agents/notifications.md`. That doc is explicit about
the cost of a new *channel*: it touches all three engines' `recipient`
functions and `cmd` wiring, not just a new package. This change deliberately
does not pay that cost yet. It builds the piece every channel needs
regardless of which engine eventually uses it — a place to register a
device's push token, and a proven way to send one push through it — and
stops there. Wiring push into `notify`/`reminder`/`nudge` as a fourth channel
is explicitly a follow-up change, once this infrastructure is verified live.

The mobile app (`freehire-mobile`) is Expo-managed (`app.json`/`eas.json`
already exist from the mobile-setup work). Expo's own Push API
(`https://exp.host/--/api/v2/push/send`) accepts a token in Expo's own format
(`ExponentPushToken[...]`) and relays to APNs or FCM depending on which
platform issued it — one integration point covers both platforms, with no
APNs `.p8` key or Firebase service account needed on freehire's side.

## Goals / Non-Goals

**Goals:**
- Store one row per registered device, reassignable when a different account
  signs in on the same device.
- A `Notifier` that reliably sends one push through the Expo Push API.
- A way to prove the whole path works end-to-end (device → token → backend →
  Expo → device) without needing a real notification-worthy event.
- Stale tokens (uninstalled app, revoked permission) don't accumulate forever
  — Expo's own receipt tells us this, so the send path prunes them.

**Non-Goals:**
- Wiring push into `notify`/`reminder`/`nudge` as a delivery channel (a
  follow-up change).
- The mobile app registering a *real* token (needs `expo-notifications` +
  permission UI, neither of which exist in `freehire-mobile` yet — also a
  follow-up, tracked separately from this backend-only change).
- Rich push payloads (images, actions, deep-link routing beyond a bare
  title/body) — the test endpoint only needs to prove delivery, not design
  the eventual notification content.

## Decisions

### 1. `user_push_tokens` is upserted by token, not user-scoped-unique

A push token identifies one app installation on one device, not one user —
Expo mints it per install. `POST /me/push-tokens` upserts
`ON CONFLICT (token) DO UPDATE SET user_id = excluded.user_id, last_seen_at =
now()`, so if a different account signs into the app on the same phone, the
row's `user_id` flips to the new owner instead of leaving a second row where
the old owner keeps a live token to a device they're no longer signed into.

**Alternative considered:** unique on `(user_id, token)`, allowing the same
token to appear under multiple users simultaneously. Rejected — that would
mean a former user of a shared/resold device keeps receiving that device's
pushes after signing out, which is both a privacy leak and simply wrong
(only whoever is currently signed in on that install should get pushes to
it).

### 2. The Notifier prunes tokens Expo reports as dead

Expo's push send response is a per-message receipt array; a token can come
back `status: "error"` with `details.error` in
(`DeviceNotRegistered`, `InvalidCredentials`, ...). `DeviceNotRegistered`
specifically means the OS itself revoked the token (app uninstalled,
permission pulled) — Expo will never successfully deliver to it again, so
the `Notifier` deletes that row on that specific error rather than leaving a
permanently-dead token to retry forever. Other error kinds are surfaced as an
error and the token is left alone (they may be transient).

### 3. The test-send endpoint only ever targets the caller's own token(s)

`POST /me/push-tokens/test` takes no destination parameter — it looks up the
authenticated caller's own registered token(s) and sends to those. There is
no way to make it push to an arbitrary token or arbitrary user; that would be
a spam/harassment vector disguised as a diagnostic endpoint. Verifying the
path for a *different* user's device (support debugging) is out of scope
here — this endpoint is "does my own push work," not an admin tool.

### 4. No platform-specific payload branching

The Expo Push API accepts one message shape (`to`, `title`, `body`, plus
optional `data`/`sound`/etc.) regardless of whether the destination token is
iOS or Android — Expo's relay handles the APNs/FCM shape difference on its
side. `platform` (`ios`/`android`) is still stored on the row, supplied by
the client at registration time, purely for our own visibility (e.g. "how
many Android installs have push enabled") — it plays no role in how a
message is sent.

## Risks / Trade-offs

- **[Risk]** Expo's relay is a dependency freehire doesn't control — an Expo
  outage means no push deliverable through this path, with no direct-APNs
  fallback. → **Mitigation**: accepted for this stage; direct APNs/FCM
  integration is a larger, separate undertaking not justified until push
  volume/reliability requirements are known. `internal/pushnotify`'s
  `Notifier` interface keeps that swap possible later without touching
  callers.
- **[Risk]** A token can be upserted to a new `user_id` while the previous
  owner is mid-flight on some other request; this is expected and matches
  how the equivalent problem is already handled for the session cookie
  (`token_version`) — nothing here introduces a new class of race.
- **[Trade-off]** No delivery-failure retry/backoff at this layer (unlike
  `notify`'s claim/lease/dead-letter machinery) — this change ships a
  synchronous send with a single Expo round trip; retry semantics belong to
  whichever engine adopts push as a channel later, matching how `email-notify`
  and `telegram-notify` also carry no retry logic of their own (the engines
  own that).

## Migration Plan

New table (`migrations/0085_user_push_tokens.sql`), additive only — no
existing table or endpoint changes. No backfill needed (empty on creation).
Rollback is dropping the table; nothing else depends on it yet.
