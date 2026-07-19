## Why

Users save jobs they intend to apply to, but there is no nudge to come back — saved
jobs quietly go stale and the vacancy may close before the user acts. A gentle,
opt-in reminder over channels the user already trusts (Telegram, email) closes the
loop between "saved" and "applied" without becoming spam.

## What Changes

- Add a **saved-job reminder** capability: when a user saves a job, a one-shot
  reminder can be scheduled to fire after a delay, delivered over the user's chosen
  channels (Telegram / email), reusing the existing `internal/notify` delivery engine.
- **Hybrid scheduling model:** an account-level default rule (enabled + default delay
  + channel selection) governs new saves; each save can override the delay or opt out
  for that one job at save time.
- **Non-intrusive save affordance:** after saving a job the UI offers quick reminder
  choices (default / tomorrow / +week / don't remind) — no modal, no forced decision;
  ignoring it leaves the account default in effect.
- **Per-reminder management on `/my/activity`:** each saved job shows its pending
  reminder ("in 3 days") with inline controls to reschedule or turn it off, plus an
  account-level reminder settings block to toggle the feature, set the default delay,
  and pick channels.
- **One-shot semantics:** a reminder fires once. It is auto-cancelled if the user marks
  the job `applied`, unsaves it, or the job closes before the fire time.
- Add a run-once **`cmd/remind`** worker (mirroring `cmd/notify`) that fires due
  reminders on a schedule and records delivery in a ledger for dedup and retries.

## Capabilities

### New Capabilities
- `saved-job-reminders`: scheduling, cancellation, and delivery of one-shot reminders
  for saved-but-not-applied jobs, with an account-level default rule and per-job
  overrides, delivered via the existing notification channels.

### Modified Capabilities
<!-- None: reminders reuse email-notify / telegram-notify channels and the user-job-tracking
     save flow without changing their existing requirements. -->

## Impact

- **DB:** new `job_reminders` table (the schedule + delivery ledger) and a
  `reminder_settings` table (or columns on `users`) for the account-level default rule;
  new sqlc queries.
- **Backend:** new `internal/reminder` package (settings + scheduling use case and the
  firing engine, mirroring `internal/subscription` + `internal/notify`); reminder settings
  + per-job reminder HTTP endpoints under `/api/v1/me`; the save/apply/unsave handlers
  orchestrate `reminder.Service` alongside `jobtracking` (which is left untouched); job
  closure is handled lazily by the worker's fire-time re-check.
- **Worker:** new `cmd/remind` run-once binary, scheduled on prod like the other cron
  workers; reuses `notify.Router` (Telegram + SES) and `worker.Bootstrap`/`ExitCode`.
- **Frontend:** save affordance on job rows/cards, reminder controls + settings block on
  `web/src/routes/my/activity/`.
- **Reused as-is:** `internal/notify` delivery/ledger pattern, `internal/telegramnotify`,
  `internal/emailnotify`, `telegram_links`, `users.email`.
