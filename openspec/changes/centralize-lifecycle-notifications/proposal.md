## Why

Notification logic for a tracked application is scattered: `internal/reminder`
nudges a candidate to apply on a saved job, but nothing pushes a nudge when an
application goes quiet past its stage's tolerated silence (`userjob.SilenceStateFor`
today only feeds a passive board badge) or when a stage moves into `interview`
(the mock-interview preset has to be found and started manually). There is no
single place deciding "given this application's stage and how long it's sat there,
what should we tell the candidate, and through which channel" — see
[GitHub #1540](https://github.com/strelov1/freehire/issues/1540). Full design in
[design.md](design.md).

Building this on the existing settings shape would also replicate a granularity
nobody uses: a production check of `reminder_settings` found exactly one real user
besides the developer account has ever enabled reminders, and they never touched the
delay-days picker or channel choice. This change simplifies that surface at the same
time, rather than carrying it into the two new nudge kinds.

## What Changes

- Add a new decision engine, `internal/nudge`, that watches `application_events`/
  `user_jobs` and drives two new one-shot nudges: **follow-up** (application gone
  silent past its stage's threshold) and **interview-prep** (stage moved to
  `interview`). MATCH→DELIVER shape, mirroring `internal/notify`; dedup via an
  "episode key" (the fact that must change for a re-notify to be warranted) instead
  of a snooze or notified-count.
- Extract the account-level notification gate into its own capability,
  `notification_settings` (renamed from `reminder_settings`, drops
  `default_delay_days`): one `enabled` flag + one channel set governs saved-job
  reminders **and** both new nudge kinds. **BREAKING**: new accounts now default to
  `enabled: true, channels: [email]` instead of disabled — opt-out, not opt-in.
  Existing rows keep their explicit values untouched.
- **BREAKING**: remove per-job reminder management entirely — `ReminderChip.svelte`,
  `reminder.Override`, the reschedule/cancel API (`PATCH`/`DELETE
  /api/v1/jobs/:slug/reminder`), and the delay-days picker on the settings page. A
  saved job always schedules at the fixed `DefaultDelayDays`; there is no per-job
  opt-out or custom delay any more.
- Move the notification settings UI off `/my/activity` onto a new account-nav
  section, `/my/notifications`.
- New `cmd/nudge` worker (run-once, own systemd timer), delivering over the existing
  email/Telegram transports with per-kind copy — no new delivery channel.

## Capabilities

### New Capabilities
- `lifecycle-nudges`: the `internal/nudge` decision engine — follow-up and
  interview-prep nudges, MATCH/DELIVER, episode-key dedup, re-check-before-send.
- `notification-settings`: the single account-level enabled+channel gate shared by
  saved-job reminders and lifecycle nudges, its default-enabled-for-new-accounts
  behavior, and its `/my/notifications` UI.

### Modified Capabilities
- `saved-job-reminders`: removes the "Account-level reminder default rule" and
  "Per-job reminder management" requirements (superseded by `notification-settings`
  and the per-job-control removal); "Scheduling a reminder on save" drops the
  override parameter — every save schedules at the fixed default delay, gated only
  by the shared `notification_settings.enabled`.

## Impact

- **Migration**: rename `reminder_settings` → `notification_settings`, drop
  `default_delay_days`; new `application_nudges` ledger table.
- **Go**: new `internal/nudge` package + `cmd/nudge`; `internal/reminder` loses
  `Override`, `Repository.RescheduleReminder`/`CancelReminder`-from-chip paths, and
  the `DelayDays` field on `Settings`; `internal/handler/user_jobs.go` loses the
  `reminder` field on save and the per-job reminder endpoints.
- **SPA**: `ReminderChip.svelte` deleted; `ReminderSettings.svelte` simplified and
  relocated to a new `/my/notifications` route; `accountNav.ts`/`accountNavIcons.ts`
  gain an entry; `api.ts` loses `rescheduleReminder`/`cancelReminder` and the
  `reminder` param on `saveJob`.
- **Ops**: new systemd timer for `cmd/nudge`.
