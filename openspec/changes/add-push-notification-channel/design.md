## Context

Three independent cron-driven engines already share a delivery-channel vocabulary
(`internal/notify` for saved-search subscriptions, `internal/reminder` for saved-job
nudges, `internal/nudge` for application lifecycle nudges), each with its own
`Notifier` interface, `Router`, and `recipient()` resolver, implemented today for
`telegram` and `email` only (see `docs/agents/notifications.md`). `internal/pushnotify`
already exists as a standalone Expo-relay transport (`Send(ctx, token, title, body)
error`), used only by the mobile app's self-test button
(`POST /me/push-tokens/test`) — it has never been wired into any delivery engine.
The mobile app (`freehire-mobile`, sibling repo) registers a device's Expo token on
sign-in / notification-toggle and stores it in `user_push_tokens`, keyed by
`user_id` (a user may have several devices).

## Goals / Non-Goals

**Goals:**
- Deliver all three notification kinds (subscription digest, reminder, nudge) to a
  user's registered mobile device(s) as a push, alongside their existing
  Telegram/email options.
- Reuse the existing per-engine `Notifier`/`Router`/`recipient()` shape rather than
  introducing a new abstraction — push becomes a third implementation of a pattern
  that already has two.
- Fan out to every device a user has registered, since push (unlike a Telegram chat
  or an email address) is not a single fixed destination per user.
- Let a tapped push (where there is exactly one relevant job) deep-link straight to
  that job in the mobile app.

**Non-Goals:**
- Collapsing `notify`/`reminder`/`nudge` into one shared engine. `internal/notify`'s
  own docs defer that explicitly until a third *channel* shows which half
  generalises; this change lands that third channel but treats each engine's
  dispatch code as still independent, per the existing documented plan.
- A deep-link target for a subscription digest with more than one matched job (no
  in-app saved-search or multi-job screen exists yet). Tapping such a push just
  opens the app.
- Any change to `TestPushToken`'s existing behavior beyond the `Send` signature
  update it must follow; folding it onto the new `SendToDevices` helper is a
  same-shape refactor left for later, not required for this change to be correct.
- Push-specific rate limiting / quiet hours — inherits whatever cadence each engine
  already runs at (cron interval), same as Telegram/email today.

## Decisions

### 1. Destination model: `dest` is the user id, not a device token

Telegram's `dest` is a chat id; email's is an address — both are one fixed value per
user. Push has zero-to-many values (devices), so `recipient()` in each engine
resolves `dest = strconv.FormatInt(userID, 10)` for the `push` channel, and the
`PushNotifier` implementation is the one that expands that into N device sends. This
keeps the `Notifier` interface itself unchanged in shape (`Send(ctx, channel, dest,
message) error`) — only its *meaning* for one channel differs, exactly as the
existing doc comment already anticipates ("Telegram resolves dest as a chat_id
string; webhook/email as a URL/address").

**Alternative considered:** resolve one `(user, token)` pair per device at the
`recipient()`/query layer, producing N separate deliveries per user like N separate
subscriptions. Rejected: it would multiply `Stats.Delivered`/`Failed` counts by
device count, break the "one digest per subscription" accounting the doc explicitly
calls out as a guarantee, and require restructuring the claim/lease/dedup ledger
(keyed on subscription/reminder/nudge id, not device) for no behavioral gain.

### 2. Soft-skip via a live "has device" column, mirroring Telegram

`GetSubscriptionForDeliveryRow`, `GetReminderForDeliveryRow`, and
`GetNudgeForDeliveryRow` each gain a `HasPushDevice bool` column
(`EXISTS(SELECT 1 FROM user_push_tokens WHERE user_id = ...)`), read the same way
`TelegramChatID.Valid` already is. `recipient()`'s new `case ChannelPush` returns
`("", false)` when it's false — an unregistered user soft-skips push precisely the
way an unlinked Telegram chat soft-skips today, keeping `Stats.SoftSkips` accurate
without asking the `Notifier` to distinguish "no destination" from "send failed."

**Alternative considered:** let `PushNotifier.Send` discover zero devices itself and
return a sentinel error the delivery loop treats as a soft skip. Rejected: every
delivery loop's `errors.Is(err, ErrChannelNotConfigured)` check is about the *whole
channel* being unconfigured (e.g. no `TELEGRAM_BOT_TOKEN`), not one recipient's
missing destination; overloading it, or adding a second sentinel, means every call
site has to know about a push-specific carve-out. A query-level boolean keeps the
existing two-tier soft-skip model (channel-level via `Router`, recipient-level via
`recipient()`) intact.

### 3. Multi-device fan-out lives in `internal/pushnotify`, not per engine

`pushnotify.SendToDevices(ctx, notifier Notifier, tokens []string, title, body
string, data map[string]string) error` is the one place that loops tokens, calls
`Notifier.Send` per token, and aggregates: **nil** if at least one device received
it, otherwise the last error. Each engine's own `PushNotifier` only renders its
message type into `(title, body, data)` and calls this helper — the fan-out/pruning
mechanics are written once instead of three times.

A device that comes back `ErrTokenPruned` is not "sent" but is not treated as the
delivery's failure either if *another* device on the same account succeeded — the
per-user outcome is "did this person receive it," same spirit as
`TestPushToken`'s existing `sent+pruned+failed=devices` accounting, just collapsed
to a single bool/error for the delivery loop's purposes. If *every* device is
pruned/failed, `SendToDevices` returns an error; the engine's delivery loop treats
that one pass as `Failed` and retries. By the next pass, the pruned tokens are gone
from `user_push_tokens`, `HasPushDevice` is false, and delivery cleanly soft-skips —
self-healing without new failure-tracking state.

**Alternative considered:** put the fan-out loop inside each of the three new
`PushNotifier`s. Rejected as pure duplication — the loop, the aggregation rule, and
the pruned-vs-failed distinction are identical in all three; only the
title/body/data rendering differs.

### 4. Deep link: `data` payload carries a job slug only when there is exactly one

`pushnotify.Notifier.Send` gains a `data map[string]string` parameter. Each engine
sets `data["slug"] = job.Slug` only when the message concerns exactly one job
(always true for `reminder` and `nudge`; true for `notify` only when
`Digest.Total == 1`). `freehire-mobile` gains a notification-response listener
(`Notifications.addNotificationResponseReceivedListener`, wired near the existing
`usePushNotifications` hook) that reads `response.notification.request.content.data
.slug` and, if present, calls `router.push('/jobs/' + slug)` (expo-router, matching
the existing `src/app/jobs/[slug].tsx` route); if absent, the tap just foregrounds
the app on its default screen — this is expo-router/OS default behavior, so no
extra code is needed for that path.

**Alternative considered:** always include a link, falling back to a synthetic
"jobs list" deep link for N>1 digests. Rejected per the proposal's explicit scope
boundary — no in-app screen exists yet to land such a link on, and building one is
a separate, user-deferred piece of work ("если много — потом").

### 5. `data` is a **BREAKING** in-repo interface change, not additive

Adding a required `data map[string]string` parameter to `pushnotify.Notifier.Send`
breaks its one existing caller (`TestPushToken` in
`internal/handler/me_push_tokens.go`), which is updated to pass `nil`. This is
preferred over an additive `SendWithData` sibling method, since `pushnotify` has
exactly one production implementation (`ExpoNotifier`) and one caller today — a
second method would be permanent surface for a distinction (with/without data) that
every future caller will want anyway.

### 6. No environment gate for the push channel

`cmd/notify`, `cmd/remind`, `cmd/nudge` register Telegram/email conditionally on
`TELEGRAM_BOT_TOKEN` / `AWS_REGION`+`NOTIFY_EMAIL_FROM` because those channels need
server-held credentials. Expo's relay needs none (the APNs/FCM credential lives on
Expo's side, configured once via `eas credentials` — already done for
`freehire-mobile`, see the mobile-push memory this session produced). The push
notifier is therefore constructed unconditionally in all three `cmd` mains; a
recipient with zero registered devices still soft-skips per-user via decision #2,
so there is no "feature globally off" state to represent.

## Risks / Trade-offs

- **[Risk] A user with many stale/uninstalled-app tokens burns one `Failed` pass
  before self-healing.** → Mitigation: decision #3's self-healing behavior bounds
  this to one failed attempt per stale cohort, not a permanent failure; acceptable
  given `MaxAttempts` dead-lettering already exists for exactly this shape of
  transient failure.
- **[Risk] `freehire-mobile` is a separate repo with its own release cadence
  (EAS build), so the backend can start sending push before the tap handler ships.**
  → Mitigation: an untapped push still delivers and foregrounds the app correctly
  (OS default) even without the new listener — the listener only upgrades "opens
  the app" to "opens the right job." Backend and mobile tasks are independently
  shippable in either order.
- **[Trade-off] Push digest copy (name + count) carries less information than the
  Telegram/email digest (up to 20 jobs with salary/company).** → Accepted: a push
  is a glance/attention channel, not a reading surface; the deep link (or opening
  the app) is where the user gets full detail, matching how the existing
  Telegram/email copy already treats its own link as "read more here."

## Migration Plan

No database migration. Sqlc query changes only (regenerate via `make sqlc`). Deploy
order is not load-bearing: the new `push` channel value only becomes reachable once
the web UI ships the chip that lets a user select it, and an old binary ignores a
channel value it doesn't recognize (falls through `recipient()`'s default case,
already the existing behavior for any unhandled channel).

## Open Questions

None outstanding — architecture, content, deep-link behavior, and scope boundaries
were confirmed with the user before this change was proposed.
