## Why

`add-push-notification-channel` (merged, #1841) gave three notification engines
(saved-search subscription digests, saved-job reminders, application nudges) a
mobile push channel, but a push is transient — miss it and it's gone, and a user
with push disabled or a saved-search-only channel has no way to browse what they
were told at all. There is no in-app record of any notification ever sent, on
either web or mobile.

## What Changes

- A new `user_notifications` table records one row per delivery EVENT (not per
  channel) at the moment `internal/notify`, `internal/reminder`, or
  `internal/nudge` marks something delivered — carrying a kind, human-readable
  title/body (reusing the short copy already written for push), an optional
  job reference (public slug, no internal id leaked), and a `read_at` for
  unread state.
- New cookie-only endpoints: `GET /api/v1/me/notifications` (offset/limit,
  matching this codebase's dominant `/me/*` pagination idiom), `POST
  /api/v1/me/notifications/:id/read`, `POST /api/v1/me/notifications/read-all`
  — mirroring the existing Gmail inbox's exact read/read-all shape.
- Web: a bell icon with an unread-count badge in the header, opening a list of
  notification cards.
- `freehire-mobile` (separate repo, tracked here as a companion task): a bell
  icon with a badge next to the existing account icon in the feed's header row,
  opening a new screen with the same cards; tapping one deep-links via the
  existing `/jobs/[slug]` route where a job reference exists.

Explicitly out of scope: deduplicating `internal/notify`'s per-channel delivery
events (a user subscribed on both Telegram and push to the same saved search
gets two near-identical entries for one match) — accepted as a known limitation,
not fixed here. See design.md's Non-Goals.

## Capabilities

### New Capabilities
- `notification-center`: the `user_notifications` ledger, its read/unread API,
  and the write-side hook into all three delivery engines, plus the web and
  mobile UI that reads it.

### Modified Capabilities
(none — this only adds a side effect at each engine's existing delivery point,
it does not change what `filter-subscriptions`/`saved-job-reminders` themselves
require)

## Impact

- **Backend (`hire`)**: new migration (`user_notifications`), a new
  `internal/notifhistory` package (or similar — see design.md) with the
  recording call, a call-site addition in each of `internal/notify`'s
  `deliverOne`, `internal/reminder`'s `fire`, `internal/nudge`'s `fire`, a new
  `internal/handler` file for the three endpoints, sqlc query additions.
- **Web**: a new bell/notifications component in the header chrome, a list
  view.
- **Mobile (`freehire-mobile`)**: a bell icon in `src/app/index.tsx`'s header
  row, a new screen route, API client additions.
- No changes to `internal/pushnotify` or the push-delivery paths themselves —
  this is a parallel read-side feature, not a fourth channel.
