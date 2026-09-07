## Context

See `proposal.md` for motivation. The constraints that shape this design:

- **Layering forbids a direct call.** `internal/application/autoapply` (block
  `application`, layer 6) may not import `internal/engage/nudge` (block `engage`,
  layer 7 — strictly above, per `internal/platform/arch/layering`). A notification
  triggered from the worker's own write path is therefore not an option; it must be
  event-sourced — a later pass reads state the worker already persisted, the same
  shape `internal/engage/nudge` already uses for its three existing kinds.
- **A successful submission deletes its own queue row.** `cmd/auto-apply/store.go`'s
  `Submit` (lines 78–101) writes `application_events(kind='applied')` via
  `MarkJobApplied` and deletes the `auto_apply_queue` row in the same transaction.
  "Submitted" can only be read from the event afterward, never from the queue table.
- **`blocked_at`/`failed_at` are write-once and permanently terminal.**
  `ClaimAutoApplyBatch` and the partial index `auto_apply_queue_claimable_idx`
  (migration 0116) exclude any row once either column is set, and nothing ever
  clears or re-sets either for the same row. A scan of `WHERE blocked_at IS NOT
  NULL` (or `failed_at`) can therefore never observe a second, different transition
  for the same row.
- **`internal/engage/nudge` already is the right mechanism.** Three kinds
  (`follow_up`/`interview_prep`/`job_closed`) already do MATCH-scans-state,
  DELIVER-over-configured-channels, dedup by `(user, job, kind, episode_key)`, with
  email/Telegram/push transports and an in-app notification-center record.

## Goals / Non-Goals

**Goals:**

- Notify the candidate over their configured channels, and record an in-app
  notification, when an auto-apply attempt ends in submission, a permanent block,
  or a dead-lettered failure.
- Reuse `internal/engage/nudge`'s existing dedup, routing, and delivery machinery
  rather than building anything new.
- Make "an auto-apply submission" a stated, queryable fact (a dedicated event
  source) rather than an accidental one.

**Non-Goals:**

- Real-time or sub-30-minute delivery. `cmd/auto-apply` itself only runs every 5
  minutes, so a same-pass notification was never actually reachable; the existing
  `freehire-nudge.timer` cadence (30 min) is consistent with the "same-day" tone the
  rest of `nudge` already sets.
- Surfacing the internal `last_error`/`SidecarResult.Reason` diagnostic string in
  the notification body. `internal/application/autoapply/status.go` already draws
  this line for the in-app drawer; this change does not cross it.
- Changing `JobDrawer.svelte`'s existing blocked/failed copy, or anything else on
  the frontend — see the last decision below.

## Decisions

### Three separate nudge kinds, not one combined kind

`auto_apply_submitted`, `auto_apply_blocked`, `auto_apply_failed` stay separate,
matching the existing one-kind-one-meaning convention (`follow_up` /
`interview_prep` / `job_closed` are three kinds for three conceptually close but
distinct events) and the fact that `JobDrawer.svelte` already gives blocked and
failed distinct copy. The alternative — one combined "auto-apply stopped" kind —
would need `Message` to carry a reason field and every render function to branch on
its presence, which is more conditional logic than three more `case`s in each
existing switch.

### Event-sourced MATCH, not a direct call from the worker

Rejected: having `cmd/auto-apply` call into `nudge` directly when it resolves an
attempt. Blocked by the block-layering rule above — `application` may not import
`engage`. This also keeps `nudge` the single owner of dedup/routing/quiet-hours
logic; `cmd/auto-apply` never needs to know a notification exists.

### `SourceAutoApply` as a new `appevent` source

`cmd/auto-apply/store.go:93` currently passes `appevent.SourceSystem` to
`MarkJobApplied`. That value is shared with unrelated system writes (e.g.
`internal/engage/nudge`'s own job-closed auto-expire), so `kind='applied' AND
source='system'` being unique to auto-apply today is an accidental invariant, not a
stated contract. Adding `appevent.SourceAutoApply = "auto_apply"` (to the `Sources`
slice in `internal/application/appevent/appevent.go`, validated by the existing
Go-side `ValidSource` — `application_events.source` is a plain `text` column with
no database CHECK constraint, so no migration is needed for this part) turns the
new MATCH query's `WHERE source = 'auto_apply'` into a real contract instead of a
coincidence.

### No `notified_at` guard column on `auto_apply_queue`

Considered and rejected: adding a `notified_at timestamptz` column to
`auto_apply_queue` to mark a blocked/failed row as "already notified". Unnecessary
because `blocked_at`/`failed_at` are provably write-once for a given row (see
Context) — the existing `application_nudges` unique index on `(user_id, job_id,
kind, episode_key)` with `episode_key = blocked_at` (or `failed_at`) already makes
re-scanning the same row on every MATCH pass a no-op via `ON CONFLICT DO NOTHING`,
identical to how `ListInterviewPrepCandidates` dedupes today.

### `actionable()` returns unconditionally true for the three new kinds

`follow_up`/`interview_prep`/`job_closed` each re-derive live state in
`Runner.actionable` because their triggering condition can lapse between MATCH and
DELIVER (a reply arrives, an interview stage is left, e.g.). None of the three new
conditions can lapse — an `applied` event, a `blocked_at`, a `failed_at` are all
permanent once written — so each new case is simply `return true`, gated only by
the `NotificationsEnabled` check every kind already goes through ahead of the
switch. `GetNudgeForDelivery` needs no change: it already joins
`jobs`/`applications`/`notification_settings`/`users` generically off `(user_id,
job_id)`, independent of kind, and already returns everything a new kind's message
needs.

### Frontend: zero changes

`RecordNotification`'s `title`/`body` are rendered server-side (`renderNudgeBatch`
in `push.go`) and stored verbatim on `user_notifications`; `NotificationCard.svelte`
renders whatever kind it is handed generically from that stored copy, and already
deep-links to `/my/tracking/[id]` via `public_slug` when a batch names exactly one
job — true for all three new kinds, since one attempt is always one job. No new
case, template, or route is needed on the frontend.

## Data model

Migration widens `application_nudges.kind`'s CHECK (`application_nudges_kind_check`,
currently `follow_up`/`interview_prep`/`job_closed` per migrations 0083/0084) to add
the three new values. No other migration: `auto_apply_queue` and
`application_events` need no schema change.

## MATCH queries (new, in `internal/platform/db/queries/nudges.sql`)

```sql
-- ListAutoApplySubmittedCandidates: application_events rows auto-apply itself
-- wrote on a successful, unattended submission.
SELECT ev.user_id, ev.job_id, ev.occurred_at
FROM application_events ev
JOIN notification_settings ns ON ns.user_id = ev.user_id AND ns.enabled
WHERE ev.kind = 'applied'
  AND ev.source = 'auto_apply'
  AND ev.retracted_at IS NULL
  AND ev.job_id IS NOT NULL
  AND ev.occurred_at > now() - make_interval(days => sqlc.arg(window_days)::int);

-- ListAutoApplyBlockedCandidates: queue entries permanently parked.
SELECT q.user_id, q.job_id, q.blocked_at
FROM auto_apply_queue q
JOIN notification_settings ns ON ns.user_id = q.user_id AND ns.enabled
WHERE q.blocked_at IS NOT NULL
  AND q.blocked_at > now() - make_interval(days => sqlc.arg(window_days)::int);

-- ListAutoApplyFailedCandidates: queue entries dead-lettered after retries.
SELECT q.user_id, q.job_id, q.failed_at
FROM auto_apply_queue q
JOIN notification_settings ns ON ns.user_id = q.user_id AND ns.enabled
WHERE q.failed_at IS NOT NULL
  AND q.failed_at > now() - make_interval(days => sqlc.arg(window_days)::int);
```

All three windowed by one new shared `Config.AutoApplyOutcomeWindowDays` field
(mirrors `FollowUpWindowDays`/`InterviewPrepWindowDays`/`JobClosedWindowDays` —
guards a first deploy against detonating the historical backlog). `nudge.Store`
gains the three matching methods; `Runner.match` gains three loops shaped exactly
like the existing `KindInterviewPrep` loop, each calling `RecordNudge` with
`EpisodeKey` set to the row's own `occurred_at`/`blocked_at`/`failed_at`.

## Rendering

Three new `case`s, same shape as the existing `KindJobClosed` ones, in:

- `push.go`: `renderNudgeBatch` / `renderNudge` (title/body for push + the in-app
  notification-center record, which reuses this same rendering).
- `transports.go`: `batchHeadline`, `renderOne` / `render`, `batchCopy` (Telegram +
  email). `batchDestination` needs no new case — its existing fallback
  (`/my/tracking`, "Open your tracking board") is correct: these three kinds are
  about an application, same as `follow_up`/`interview_prep`, unlike `job_closed`'s
  override to `/my/activity`.

No `Message` struct field is needed — none of the three kinds carries kind-specific
display data the way `follow_up`'s `DaysSilent` does.

## Risks / Trade-offs

- **[Risk] A candidate on the free tier who somehow still has a standing
  `auto_apply_queue` row (e.g. downgraded mid-attempt) gets notified about a
  feature they can no longer use.** → Mitigation: out of scope for this change —
  the existing `blocked`/`failed` terminal outcomes already stop retrying
  regardless of plan tier, and the notification is reporting a fact ("this attempt
  ended"), not offering the feature again. No different from today's `/my/tracking`
  page, which shows the same terminal card to a lapsed-Pro user.
- **[Risk] Latency.** Worst case ~30 minutes from outcome to notification (the
  `freehire-nudge.timer` cadence). → Mitigation: accepted — see Non-Goals. Matches
  the tone of the other three kinds today.
- **[Trade-off] `SourceAutoApply` changes behavior of one existing write path**
  (`cmd/auto-apply/store.go:93`) rather than being purely additive. → Mitigation:
  the change is narrowly scoped to one call site's constant argument; nothing reads
  `appevent.SourceSystem` today expecting auto-apply's `applied` events specifically
  (confirmed: `SourceSystem` is otherwise only used by `nudge`'s unrelated
  job-closed auto-expire, which writes `kind='stage_set'`, not `kind='applied'`), so
  no existing query's result set changes.

## Migration Plan

1. Add migration widening `application_nudges_kind_check`.
2. Add `appevent.SourceAutoApply`; change `cmd/auto-apply/store.go`'s one call site.
3. Add the three SQL queries; run `make sqlc`.
4. Extend `nudge.Store`, `nudge.Config`, `Runner.match`, `Runner.actionable`, and the
   rendering functions.
5. Ship as one deploy — no new deploy artifact, no timer change, no feature flag:
   the next `freehire-nudge.timer` tick after deploy starts matching any
   already-existing terminal `auto_apply_queue` rows or `applied` events within the
   new window, which is the intended backfill-free rollout (mirrors how
   `job_closed` shipped).

Rollback: revert the deploy. The widened CHECK constraint and the new
`SourceAutoApply` values left in `application_events`/`application_nudges` are
inert to every other reader if the code is rolled back (nothing else queries
`source = 'auto_apply'` or the three new kinds).

## Open Questions

(none — all decisions above were confirmed during design review)

## Post-implementation fixes (code review)

Two corrections landed after the initial implementation, found by code review
against code this design didn't originally account for:

- **Blocked/failed are made mutually exclusive.** `AutoApplyQueueMetrics`
  (`metrics.sql`) already documents that a row can carry both `blocked_at` and
  `failed_at` (a lease-timeout race between a park and a fail) and that the
  dead-letter marker wins. `ListAutoApplyBlockedCandidates` now excludes rows
  where `failed_at IS NOT NULL`, matching that precedent — otherwise the same
  attempt could be matched as both a blocked and a failed nudge.
- **The failed-nudge copy no longer says "after retrying".** `failed_at` is also
  set by `Runner.deadLetterImmediately` (an unconfirmed submission, or a
  submission that went through but couldn't be durably recorded) — both
  first-attempt outcomes with zero retries. The copy across all channels now says
  the attempt "won't try again" instead of asserting a retry happened.
