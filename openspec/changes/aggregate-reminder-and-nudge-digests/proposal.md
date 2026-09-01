## Why

Saved-search subscriptions already arrive as one digest per pass, but the other two
notification engines still send one message per item. A user who saves eight jobs in a
day gets eight separate reminder emails three days later, and a user with five silent
applications gets five nudges in one 30-minute pass. The flood is what makes people
disable notifications outright, which costs us every future alert too, not just the
noisy ones.

## What Changes

- Due saved-job reminders are grouped by user and delivered as **one message per user
  per pass**, on every channel (email, Telegram, push), instead of one message per job.
- Lifecycle nudges are grouped by `(user, kind)` and delivered as **one message per
  group**. The three kinds (`follow_up`, `interview_prep`, `job_closed`) stay in
  separate messages — they are different conversations and read badly merged.
- A reminder's `fire_at` is rounded forward to the account's daily notification hour
  (`notification_settings.digest_time` in the account timezone, defaulting to 09:00 and
  UTC when unset), so everything saved on one day becomes due in a single pass rather
  than scattered across the 15-minute worker cadence. **BREAKING** for exact timing: a
  reminder now lands at the account's notification hour on or after the 3-day mark, not
  exactly 72 hours after the save.
- A grouped delivery records **one** in-app notification carrying the job list in
  `user_notifications.jobs` (the existing JSONB column, already rendered by
  `/my/notifications/[id]/jobs`) instead of one row per job. A single-item group keeps
  today's shape: `public_slug` set, `jobs` NULL.
- Each engine's `Notifier` seam becomes batch-shaped (`[]ReminderMessage` /
  `[]Message`), so a channel renders a list and not a single job.

Out of scope: merging the three notification engines into one worker, and any change to
`internal/engage/notify` (saved-search digests already batch).

## Capabilities

### New Capabilities

- `notification-digest-batching`: the grouping rule shared by the reminder and nudge
  delivery engines — what forms a group, how many items a message itemizes versus
  carries, what the in-app record looks like, and how a failed group send is retried.

### Modified Capabilities

- `saved-job-reminders`: the scheduled fire time is rounded forward to the account's
  daily notification hour, and delivery of a due batch is one message per user rather
  than one per reminder.

## Impact

- `internal/engage/reminder` — `Service.fireAt` (bucketing), `Runner.Run`/`fire`
  (grouping), `Notifier`/`Router` signature, email/Telegram/push transports, repository
  (one new read for `digest_time` + `users.timezone`).
- `internal/engage/nudge` — `Runner.deliver`/`fire` (grouping by `(user, kind)`),
  `Notifier`/`Router` signature, transports.
- `internal/platform/db/queries/reminders.sql` — one new query for the scheduling
  context; regenerated via `make sqlc`.
- No schema migration: `user_notifications.jobs` already exists (migration 0091).
- `docs/agents/notifications.md` — the batching rule and the two bounds belong in the
  always-true list.
