## Context

Three notification engines share one channel vocabulary but not one delivery shape.
`internal/engage/notify` (saved-search digests) already batches: it groups matched jobs
per subscription and sends one `Digest`. `internal/engage/reminder` and
`internal/engage/nudge` do not — each runs `for _, id := range due { fire(id) }` and each
`fire` calls `Notifier.Send` with exactly one job.

The two engines differ in how their items become due, and the difference decides the
design:

- **nudge** has no `fire_at`. `match` inserts rows and the same pass's `deliver` claims
  them, on a 30-minute timer. Everything one account has pending is therefore already in
  one pass — grouping alone fixes the flood.
- **reminder** has `fire_at = save + DefaultDelayDays`, on a 15-minute timer. Two saves
  hours apart become due in different passes, so grouping alone changes almost nothing.
  The fire times must be made to coincide first.

`docs/agents/notifications.md` records a deliberate decision not to collapse the three
engines into one, and this change does not revisit it.

## Goals / Non-Goals

**Goals:**

- One message per account per pass for reminders; one per `(account, kind)` for nudges.
- Same behaviour on email, Telegram and push.
- One in-app notification row per grouped delivery, using the JSONB column that already
  exists for exactly this.
- No schema migration.

**Non-Goals:**

- Merging the reminder, nudge and notify engines, or their workers and timers.
- Merging nudge kinds into one message.
- Any change to `internal/engage/notify`.
- Retro-bucketing the `fire_at` of reminders already pending at deploy time.

## Decisions

### Bucket `fire_at` at schedule time, not at claim time

`Service.fireAt` gains a rounding step: take `now + DefaultDelayDays`, then move forward
to the first occurrence of the account's notification hour in the account's timezone.
The claim query is untouched.

*Why over a lookahead claim* (`fire_at <= now() + window`): a lookahead makes the
delivery hour drift with the worker's own schedule and fires reminders early by an
arbitrary amount, so "3 days" quietly becomes "2.5 to 3 days". Bucketing keeps a single,
explainable rule — "at your notification hour, on or after the third day" — and the
grouping falls out of it rather than being bolted on. It also needs no new claim
semantics, which is where the `SKIP LOCKED` lease correctness lives.

*Cost:* pending rows written before deploy keep their unbucketed `fire_at` and go out
ungrouped. They drain within `DefaultDelayDays`. Backfilling them is possible but not
worth a migration for a three-day tail.

### Reuse `notification_settings.digest_time` as the notification hour

The column exists and already means "the hour I want to hear from you", but is read only
by `notify` when frequency is `daily`. Reading it here for every account — including
`instant` ones, who have it unset and get the 09:00 default — extends its meaning
slightly rather than inventing a second time-of-day setting that would inevitably drift
from it.

*Data access:* `ScheduleOnSave` currently reads `GetNotificationSettings` only, which has
no timezone. Add one sqlc query returning `digest_time` and `users.timezone` together, so
the two halves of one decision come from one read and cannot disagree.

### Make the `Notifier` seam batch-shaped

`reminder.Notifier.Send(ctx, channel, dest string, ms []ReminderMessage) error` and
`nudge.Notifier.Send(ctx, channel, dest, kind string, ms []Message) error`.

*Why over adding a parallel `SendBatch`:* a second method leaves a per-item path alive,
and the spec requires that no channel falls back to it. One batch-shaped method makes
the single-item case a slice of one and removes the ability to regress. The two engines
keep their own interfaces, as they do today — the doc's reason for that (each carries its
own payload, `ErrChannelNotConfigured` and `recipient`) is unchanged.

### The reminder batch key carries the channel set; the nudge key carries the kind

`job_reminders.channels` is snapshotted when the reminder is scheduled — migration 0034
says why: "a later rule edit never rewrites a pending reminder". It is therefore a
property of the REMINDER, not of the account, and `GetReminderForDelivery` returns it per
row. Grouping on `user_id` alone would let the first member's row decide the channels for
the whole batch, sending one reminder over another's channels and stamping it delivered
anyway. The key is `(user_id, canonical channel set)`; the set is sorted only for the key,
so the send still walks the first member's own slice in its stored order.

`internal/engage/nudge` needs no such split — `GetNudgeForDelivery` reads
`notification_settings.channels` live, which IS an account property — and instead carries
the kind, since the three kinds must not share a message.

### Group and validate in the same order as today

`Runner.Run` claims as it does now, then loads each claimed item's delivery context and
runs the existing per-item checks (`job_open`, `still_actionable`, quiet hours) BEFORE
grouping. Cancelled and deferred items leave the batch; survivors are grouped by user
(or `(user, kind)`) and sent once.

*Why check first, group second:* the checks are per-item and already correct; reordering
them would change cancellation semantics. Quiet hours is per-account, so every survivor
in a group answers it identically — checking per item costs nothing and keeps one code
path.

### Reuse `user_notifications.jobs` rather than a snapshot table

Migration 0091 already added a JSONB array of `{title, company, slug}` for this exact
purpose, and `/my/notifications/[id]/jobs` already renders it. A multi-item group writes
`jobs` and leaves `public_slug` NULL; a single-item group keeps today's shape. Nothing
new to migrate, nothing new to render.

### Two bounds, deliberately different numbers

10 itemized in a message, 200 carried in the record — the same split `notify` uses
(`ListLimit` / `SnapshotCap`), for the reason recorded in `docs/agents/notifications.md`:
they were one knob until 2026-08-21, and lowering the email's list length silently
truncated the on-site page the email linked to. `notify.ListLimit` is an exported
constant and is reused directly rather than copied. `SnapshotCap` is not — it is a field
on `notify.Config`, and each engine already carries its own `Config`, so reminder and
nudge each gain a `SnapshotCap` field defaulting to the same 200.

## Risks / Trade-offs

- **A group send failure now costs the whole group an attempt** → `MaxAttempts` is 5 and
  the failure modes are channel-wide (SES down, bot token wrong), so an outage that fails
  a group would have failed each item individually anyway. Partial-success bookkeeping
  would buy accuracy nobody reads at the cost of a second delivery ledger.
- **Reminders no longer land exactly 72h after the save** → this is the point of the
  change, but it is a visible timing shift. The spec states the new rule explicitly, and
  the settings UI already presents `digest_time` as the account's notification hour.
- **An account with a huge backlog gets one enormous message** → the message lists 10 and
  counts the rest; the record caps at 200. A backlog beyond 200 is bounded by
  `ClaimBatch` (500) per pass and drains over passes.
- **`centralize-lifecycle-notifications` is an unarchived change that modifies the same
  `saved-job-reminders` requirements this one does** → this change's delta is written
  against the CODE's current behaviour, which is that change's outcome. Archive
  `centralize-lifecycle-notifications` before archiving this one, or the older delta will
  re-introduce the per-job override text it removed.
- **Timezone-less accounts bucket to 09:00 UTC** → a mildly odd hour for some, but the
  same fallback `deliverywindow` already uses for quiet hours, so the two timing rules
  agree about who has no timezone.

## Migration Plan

No schema migration. Deploy order is the ordinary one: `make sqlc` for the new query,
then ship. Reminders pending at deploy keep their old `fire_at` and deliver ungrouped for
up to `DefaultDelayDays`; nudges group from the first pass after deploy, since they carry
no schedule.

Rollback is a plain revert — nothing written by this change is unreadable by the previous
binary: bucketed `fire_at` values are ordinary timestamps, and a multi-item
`user_notifications` row is the shape `notify` has been writing since migration 0091.

## Open Questions

None.
