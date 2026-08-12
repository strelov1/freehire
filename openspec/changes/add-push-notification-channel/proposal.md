## Why

freehire's mobile app (`freehire-mobile`, sibling repo) already registers Expo push
tokens and can receive a one-off self-test push (`POST /api/v1/me/push-tokens/test`),
but none of the three cron-driven notification engines — saved-search subscriptions
(`internal/notify`), saved-job reminders (`internal/reminder`), or application
lifecycle nudges (`internal/nudge`) — can actually deliver to it. Each already
supports two channels, Telegram and email, behind a shared `channel` vocabulary; push
is the natural third, and the mobile app is otherwise a dead end for anything the
backend already knows how to tell a user.

## What Changes

- Add `push` to the shared delivery-channel vocabulary (`notify.Channels` /
  `notify.ValidChannel`), used by all three engines' create/validate paths.
- Add a `PushNotifier` to each of `internal/notify`, `internal/reminder`,
  `internal/nudge`, rendering a short title/body from that engine's own message type
  and delivering through the existing `internal/pushnotify` Expo transport, fanned out
  to every device the recipient has registered (a user, unlike a Telegram chat or an
  email address, may have more than one).
- Extend `pushnotify.Notifier.Send` with a `data map[string]string` payload (job slug,
  for deep-linking) — **BREAKING** (in-repo interface, one call site updated).
- Add a `pushnotify.SendToDevices` helper: fans a title/body/data out to a set of
  tokens, aggregating per-recipient outcome (delivered if any device received it,
  otherwise the delivery is treated as failed and retried like any other channel).
- Extend each engine's delivery-context query (`GetSubscriptionForDelivery`,
  `GetReminderForDelivery`, `GetNudgeForDelivery`) with a live "has a registered push
  device" check, so an unregistered recipient soft-skips push exactly the way an
  unlinked Telegram chat already soft-skips today — no schema migration, since
  `channel`/`channels` are unconstrained `text`/`text[]`.
- Register the push notifier unconditionally in `cmd/notify`, `cmd/remind`,
  `cmd/nudge` — unlike Telegram/email, Expo needs no server-side credential, so there
  is no env-var gate.
- Web: add a third "Push" chip next to Telegram/Email in
  `AlertChannels.svelte` (search-subscription channel picker) and in
  `ReminderSettings.svelte` (the shared reminder+nudge "Deliver over" row).
- `freehire-mobile` (sibling repo, tracked here as an out-of-OpenSpec companion task
  — see design.md): add the notification-tap handler that reads a push's `data.slug`
  and deep-links to `/jobs/[slug]` via expo-router; without it the new channel sends
  correctly but tapping the result does nothing.

Explicitly out of scope: collapsing `notify`/`reminder`/`nudge` into one shared
engine (deferred by the existing `internal/notify` docs until it's clear which half
generalises — this change is not that trigger), and any multi-job deep-link target
for a subscription digest with more than one match (tapping opens the app with no
target screen; picking/paging through several jobs from a push is deferred).

## Capabilities

### New Capabilities
- `mobile-push-channel`: the shared push-transport plumbing (multi-device fan-out,
  deep-link data payload) and application-nudge push delivery, which has no existing
  spec home (`internal/nudge` was never given its own capability spec).

### Modified Capabilities
- `filter-subscriptions`: `channel` gains a third accepted value (`push`); a push
  digest renders as a short title/body instead of the full job listing.
- `saved-job-reminders`: the reminder rule's `channels` set gains a third accepted
  value (`push`).

## Impact

- **Backend (`hire`)**: `internal/notify`, `internal/reminder`, `internal/nudge`,
  `internal/pushnotify`, `internal/handler/me_push_tokens.go` (Send signature only),
  `internal/db` (three sqlc query files, regenerated), `cmd/notify`, `cmd/remind`,
  `cmd/nudge`.
- **Web**: `web/src/lib/components/filters/AlertChannels.svelte`,
  `web/src/lib/components/ReminderSettings.svelte`.
- **Mobile (`freehire-mobile`, separate repo/git history)**: a notification-response
  listener wired into `src/app/_layout.tsx` or `usePushNotifications.ts`, using the
  existing expo-router deep-link scheme. This repo has no OpenSpec of its own; the
  task is tracked in this change's `tasks.md` as a companion step and implemented in
  its own worktree/commit.
- No database migration (existing `text`/`text[]` columns, no `CHECK` constraint).
