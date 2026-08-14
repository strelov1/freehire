## Why

Every notification today delivers "as it happens" — `cmd/notify`,
`cmd/remind`, and `cmd/nudge` each run every few minutes and send whatever
they find, with no regard for the time of day. There is no way to batch
saved-search alerts into a daily digest, and no way to stop anything from
arriving at 3am.

## What Changes

- A new account-wide choice for saved-search alerts: `instant` (today's
  behavior) or `daily` at a chosen local time. Only `internal/notify`'s
  digest delivery reads this — reminders and nudges stay one-shot,
  trigger-bound events.
- A new account-wide quiet-hours window that defers *every* notification
  kind — search alerts, saved-job reminders, and all three nudge kinds —
  until it ends. Nothing is dropped, only delayed to the next worker pass.
  A `daily`-mode digest is exempt: a chosen delivery time is itself the
  user's preference, so quiet hours does not second-guess it.
- A `timezone` field on the account (IANA name, e.g. `Europe/Moscow`),
  editable on `/my/profile`, so "9am" and "quiet after 10pm" mean the
  user's own local time. Absent means UTC.
- New controls on `/my/notifications/settings`: frequency (instant/daily +
  time picker) and quiet hours (optional start/end).

Explicitly out of scope: per-saved-search frequency (one account-wide
setting governs all searches); merging multiple searches' daily digests into
one combined message (each keeps sending its own, just delayed).

## Capabilities

### New Capabilities
- `notification-delivery-timing`: the account's timezone, the digest
  frequency/time setting (and how `internal/notify`'s DELIVER phase gates
  on it), the quiet-hours window and how all three delivery engines
  (`internal/notify`, `internal/reminder`, `internal/nudge`) gate on it, and
  the settings/profile UI for both.

### Modified Capabilities
- `filter-subscriptions`: "Digest delivery with retry and dead-letter" gains
  the daily-mode timing gate and the quiet-hours gate as additional
  conditions on when a pending digest is claimed for delivery.
- `saved-job-reminders`: "One-shot delivery" gains the quiet-hours gate as
  an additional condition on when a due reminder is claimed for delivery.

## Impact

- **Migration**: `users.timezone text`; `notification_settings` gains
  `digest_frequency`, `digest_time`, `quiet_hours_start`, `quiet_hours_end`
  (all nullable/defaulted, additive).
- **Go**: `internal/notify`'s DELIVER claim query, `internal/reminder`'s
  claim query, `internal/nudge`'s claim query all gain the same-shaped
  time-window predicate; a small shared helper for "is now within this
  window, in this timezone" (package TBD in design — likely
  `internal/notify` or a new tiny shared package, since all three already
  depend on neither of each other).
- **SPA**: `ProfileForm.svelte` gains a timezone field; a new block on
  `/my/notifications/settings` (extending `ReminderSettings.svelte` or a
  sibling component) for frequency + quiet hours.
- **Ops**: none — no new worker, existing cron cadences unchanged.
