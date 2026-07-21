## Context

Users save jobs (`user_jobs.saved_at`) but nothing brings them back, so saved jobs go
stale and vacancies close unread. The codebase already ships a mature notification stack
we can lean on:

- `internal/notify` — a MATCH→DELIVER engine for filter subscriptions, built around a
  **dedup + delivery ledger** (`subscription_matches`: `claimed_at` / `notified_at` /
  `attempts` / `failed_at`) leased with `FOR UPDATE ... SKIP LOCKED` (the lease doubles
  as a crash reaper), driven by the run-once `cmd/notify` worker.
- Per-channel transports split into **render** (`notifier.go`) + **client**
  (`client.go`): `internal/telegramnotify` (Bot API `sendMessage`, HTML) and
  `internal/emailnotify` (AWS SES v2). Both implement `notify.Notifier`, dispatched by
  `notify.Router` keyed on channel.
- User destinations already exist: `telegram_links.chat_id` (per user) and `users.email`.
- Worker plumbing: `worker.Main` / `worker.Bootstrap` / `worker.ExitCode`.

A reminder is the same ledger-and-worker shape as a subscription, but the trigger is
"a scheduled time arrived" instead of "a new job matched a filter", and the unit is a
single saved job rather than a saved search.

The save/apply/unsave flow lives in `internal/jobtracking` (thin HTTP handlers →
service → sqlc repo over `user_jobs`). Job closure happens in the ingest/liveness paths.

## Goals / Non-Goals

**Goals:**
- One-shot, opt-in reminders for saved-but-not-applied jobs over Telegram / email.
- Hybrid scheduling: an account default rule (enabled, delay, channels) plus a per-job
  override chosen at save time.
- Non-intrusive: ignoring the save-time affordance falls back to the account default;
  the feature is off until the user opts in.
- Auto-cancel a pending reminder on apply, unsave, or job close.
- Per-job reminder visibility + reschedule/cancel on `/my/activity`.
- Reuse the existing delivery transports and worker plumbing; do not reinvent them.

**Non-Goals:**
- Recurring / escalating reminders (explicitly one-shot).
- Reminders for jobs the user only viewed but never saved.
- New delivery channels beyond the existing Telegram + email.
- Reminder analytics / open-tracking.
- Digesting multiple due reminders into one message (each reminder is its own message;
  batching is a later seam, noted below).

## Decisions

### 1. Two tables: `reminder_settings` (rule) + `job_reminders` (schedule/ledger)

`reminder_settings` — one row per user: `user_id PK`, `enabled bool`, `default_delay_days
int`, `channels text[]` (subset of `notify.Channels`), `updated_at`. Absent row = feature
never configured = disabled (matches the "off by default" requirement without a backfill).

`job_reminders` — the schedule and delivery ledger, one row per scheduled reminder:
`id`, `user_id`, `job_id`, `fire_at timestamptz`, `channels text[]` (snapshot of the
rule's channels at schedule time), `status text` (`pending` | `delivered` | `cancelled`),
plus the ledger columns mirrored from `subscription_matches`: `claimed_at`, `attempts`,
`failed_at`, `last_error`, `created_at`, `delivered_at`.

**At most one pending reminder per `(user_id, job_id)`** enforced by a partial unique
index `WHERE status = 'pending'`. Re-saving replaces via an upsert that targets that
index. `delivered` / `cancelled` rows are retained as history and do not block a future
pending row.

*Alternative considered:* store the reminder as nullable columns on `user_jobs`
(`reminder_fire_at`, `reminder_status`). Rejected — it couples the ledger's retry/attempt
bookkeeping to the core tracking row, muddies the `user_jobs` query surface, and loses the
delivered/cancelled history. A dedicated table keeps the concern isolated (per the
project's isolation principle) and mirrors the proven `subscription_matches` shape.

### 2. A standalone `internal/reminder.Service` the handlers orchestrate

Reminders are their own use case, mirroring how `internal/subscription` is a separate
package the handler wires alongside `jobtracking` rather than inside it. `reminder.Service`
owns: the settings CRUD, `ScheduleOnSave(userID, jobID, override)` (reads the rule,
resolves the effective fire time — override delay if present, else the account default when
enabled — and upserts or, on opt-out, cancels the pending `job_reminders` row), and
`Cancel(userID, jobID)`. It depends on its own narrow `Repository` (slug resolution reuses
the existing `GetJobIDBySlug`), so `internal/jobtracking` is left completely untouched.

The HTTP handlers orchestrate the two use cases: the save handler calls `tracking.SaveJob`
then `reminder.ScheduleOnSave` with the request's override; the apply and unsave handlers
call `reminder.Cancel` after their tracking action.

*Why not transactional inside `jobtracking`:* the earlier plan folded scheduling into
`SaveJob` for save+schedule atomicity. But `jobtracking`'s repository is a pure `*db.Queries`
adapter with no pool, so a shared transaction would force tx plumbing and couple job-tracking
to the reminder rule and channel vocabulary. Atomicity turns out to be unnecessary: the
worker's fire-time re-check (below) already backstops correctness, so eager scheduling and
cancellation are best-effort fast paths, not invariants that must be transactional. Keeping
reminders in their own package is the surgical, better-isolated choice.

Cancellation triggers split by cost. **Apply and unsave** are explicit user actions on one
job, so the handler eagerly calls `reminder.Cancel` (`UPDATE ... SET status='cancelled'
WHERE status='pending'` — idempotent, no-op if none). **Job closure** happens across several
scattered, sometimes bulk paths (`CloseUnseenJobs` sweep, `CloseJobBySourceExternalID`,
`CloseJobByID`, liveness), so hooking every one would couple reminders into hot ingest
paths. Instead the worker's fire-time re-check is the single authoritative enforcement
point: before sending it verifies the job is still open and still saved-but-unapplied, and
cancels-and-skips otherwise. A closed job's reminder therefore lingers as `pending`
(invisible to the due-scan until its `fire_at`) and is cancelled lazily the moment the
worker would fire it — the observable outcome ("no reminder is sent for a dead job") is
identical, without touching the close paths. The re-check also backstops apply/unsave, so a
missed eager cancel never sends a stale reminder.

### 3. New `internal/reminder` engine + `cmd/remind` worker, mirroring notify

`internal/reminder` owns the firing pass: `Runner.Run` claims due pending reminders
(`fire_at <= now()`, `status='pending'`, lease via `FOR UPDATE ... SKIP LOCKED` reclaiming
dead leases), delivers each, and marks it `delivered` — idempotent under retry because a
delivered row is never re-claimed. `MaxAttempts` dead-letters (`failed_at`) a reminder
that keeps failing. This is the `internal/notify` DELIVER stage without the MATCH stage
(reminders are pre-scheduled, so there is nothing to search).

`cmd/remind/main.go` is `cmd/notify`'s twin: `worker.Main(run)` → `worker.Bootstrap` →
build the reminder router → `Runner.Run` → `worker.ExitCode`. Scheduled on prod like the
other cron workers (e.g. every ~15 min so "tomorrow" fires within a small window). An
unconfigured feature (no channels wired) logs and exits 0.

### 4. Reminder-specific message, reused transports

Reminders render differently from subscription digests ("You saved **{title}** at
**{company}** — still interested? [Apply]"), so the engine depends on its own small
`Notifier` seam `Send(ctx, channel, dest, ReminderMessage)`. A reminder `Router` maps
channel → implementation, each reusing the **existing transport client**: the Telegram
notifier wraps `telegramnotify.Client.SendMessage`; the email notifier wraps the
`emailnotify` SES sender. Destinations resolve live at delivery (`telegram_links.chat_id`,
`users.email`); a channel with no destination is a soft-skip, not a failure.

*Alternative considered:* repurpose `notify.Digest` (one job, `SavedSearchName` holding
the reminder headline). Rejected — overloading the subscription digest shape for a
different message is exactly the "clever shim" the guidelines warn against; a dedicated
message type is clearer and the transport clients are already the reusable layer.

### 5. HTTP surface under `/api/v1/me`

- `GET/PUT /me/reminder-settings` — read/update the account default rule (cookie auth,
  channel allowlist derived from `notify.Channels`, like subscriptions).
- Per-job reminder control folded into the existing tracking surface: the save request
  carries the override; `PATCH /jobs/:slug/reminder` reschedules and `DELETE
  /jobs/:slug/reminder` cancels a pending reminder without unsaving.
- The saved-jobs listing (`GET /me/tracking?filter=saved`) is extended to project each
  row's pending reminder (`fire_at` or none) so `/my/activity` can render it.

### 6. Frontend: save affordance + settings on `/my/activity`

`SavedJobs.svelte` / `JobRow` gain an inline reminder chip ("Remind in 3 days" with
reschedule/off controls). The save action surfaces the quick choices (default / tomorrow
/ +week / don't remind) non-modally. `/my/activity` gets a reminder settings block
(enable, default delay, channels) calling the new settings endpoint.

## Risks / Trade-offs

- **Reminder fires just as the user applies elsewhere / job closes** → cancellation is
  best-effort at fire time: the worker re-checks that the job is still saved-not-applied
  and open immediately before sending, so a race between cancel and fire skips the send.
- **Clock granularity** → "tomorrow" is delay-days from save; the worker cadence (~15 min)
  bounds lateness. Acceptable for a nudge; no minute-level precision promised.
- **No batching** → a user with many due reminders gets several messages in one worker
  pass. Mitigation/seam: the ledger is keyed per `(user, job)`, so grouping due reminders
  per user into one digest is a later change without a schema shift; noted, not built now
  (YAGNI until volume warrants it).
- **`channels text[]` snapshot vs live rule** → the reminder captures channels at schedule
  time so later rule edits don't retroactively rewrite pending reminders. Trade-off:
  changing channels only affects future saves; documented behavior, matches "existing
  pending reminders are left intact" in the spec.
- **SES/Telegram outage at fire time** → same retry/dead-letter path as subscriptions
  (`attempts` / `MaxAttempts` / `failed_at`); a persistently failing reminder dead-letters
  rather than blocking the queue.

## Migration Plan

1. Additive migration: create `reminder_settings` and `job_reminders` (no changes to
   existing tables). Following the repo's initdb convention, add to the migration source;
   recreate volume in dev to apply, run manually on prod (owned by `hire`).
2. Ship backend (settings + scheduling hooks + engine) behind the natural gate: with no
   users opted in, `job_reminders` stays empty and `cmd/remind` is a no-op.
3. Schedule `cmd/remind` on prod (cron/systemd timer, ~15 min).
4. Ship the frontend affordance + settings.
5. Rollback: stop scheduling `cmd/remind` and hide the frontend; the tables are inert
   (no reminders fire). Drop tables only if fully reverting.

## Open Questions

- **Worker cadence:** ~15 min proposed — confirm against how tight "tomorrow" should feel
  vs. cron noise. (Tunable, not load-bearing for the schema.)
- **Default delay value** for a freshly enabled rule (proposing 3 days) — product choice,
  easily changed.
- **Message copy / CTA** wording and whether to include the AI match score in the
  reminder — deferred to implementation, no architectural impact.
