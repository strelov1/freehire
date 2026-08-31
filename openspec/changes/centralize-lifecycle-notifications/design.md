## Context

This document is the design: problem framing, the production-usage check that
motivated the settings simplification, and every scoping decision. It used to
point at a narrative companion under `docs/superpowers/specs/` that was never
written, so the pointer promised a document that has never existed.

Current state:
- `internal/reminder` schedules a saved-job reminder at save time (`ScheduleOnSave`,
  taking an `*Override` for a per-job delay/disable), fires due reminders from
  `job_reminders` with a re-check-at-fire ("still saved and unapplied") cancellation.
  `reminder_settings` (PK `user_id`) holds `enabled`, `default_delay_days`,
  `channels`.
- `internal/notify` is the filter-subscription digest engine: MATCH (record new
  matches in a dedup ledger) then DELIVER (claim + send). `notify.Channels`/
  `ValidChannel` is the shared channel vocabulary both `notify` and `reminder` gate
  against.
- `internal/userjob/silence.go` (`silenceThresholds`, `DaysSilent`,
  `SilenceStateFor`) is pure, I/O-free, and already the single source of truth for
  "how long has this application gone quiet" — today it only feeds a passive board
  badge.
- `internal/assistant`'s `PresetInterview` session is created client-side
  (`createRehearsal(slug)`), rendered from `JobBoard.svelte` on
  `/my/tracking/[id]`. Nothing server-side can mint a session for a notification
  link.
- A production check (2026-08-09) of `reminder_settings`: of 3 rows total, only one
  real user (besides the developer) has ever enabled reminders, and left the
  delay-days picker and channel choice at their account defaults.

## Goals / Non-Goals

**Goals:**
- One decision layer driving three nudge kinds — saved-not-applied (existing),
  gone-silent (new), entered-interview (new) — off one account-level toggle.
- Dedup that doesn't re-ping every cron pass, without a snooze or a notified-count
  column.
- Simplify the settings surface to match its actual (near-zero) usage: one toggle,
  one channel choice, no per-job or per-stage granularity.

**Non-Goals:**
- Not a rewrite of the `notify`/`reminder` `Notifier` interface split — see
  `docs/agents/notifications.md`. `internal/nudge` is a third, independently small
  engine for a third distinct use case, not a shared abstraction across all three.
- No nudge on `offer`/`rejected`/`withdrawn` — the board and mail-sync already
  surface these.
- No per-job override, no per-stage opt-out, no delay-day choice anywhere.
- No pre-created assistant session in a notification — nudges link to the tracking
  page, which already has the "Rehearse the interview" button.

## Decisions

### D1: New `internal/nudge` package (MATCH→DELIVER), not an extension of `internal/reminder`

`internal/reminder` schedules at write time (on save) and re-checks liveness at
fire time. The two new nudge kinds don't have a natural "write time" to schedule
from without hooking `internal/jobtracking`'s `TrackJob` (stage change) and the
mail-linking path (`last_activity_at` bump) and rescheduling/cancelling a pending
row on every one of those writes — exactly the fragility `reminder`'s own
cancel-at-fire design exists to avoid (see `internal/reminder/engine.go`'s
doc comment: re-checking at fire time instead of "hooking the scattered close
paths"). Instead `internal/nudge` computes candidates live each pass from current
state (`userjob.SilenceStateFor`, `application_events`), like `internal/notify`
computes subscription matches live each pass. Zero changes to `internal/jobtracking`.

**Alternative considered**: extend `internal/reminder`'s `job_reminders` table with
a `kind` column and schedule follow-up/interview-prep at write time. Rejected —
requires hooking 3 write sites (`TrackJob`, mail-linking, `MarkApplied`) to keep a
pre-computed `fire_at` correct as `last_activity_at` moves, versus zero hooks for
the live-recompute approach.

### D2: Dedup via "episode key", not a snooze or notified-count

`application_nudges` carries `UNIQUE (user_id, job_id, kind, episode_key)`. MATCH
inserts `ON CONFLICT DO NOTHING`. `episode_key` is the fact that must change before
a re-notify is warranted:
- `follow_up`: `last_activity_at` at match time. Silence dragging on longer doesn't
  change it (no repeat nudge); new inbound mail does (a fresh episode — and the
  silence state will have left `silent` anyway, so it self-filters at MATCH time
  too).
- `interview_prep`: the `occurred_at` of the `stage_set → interview` event. A later
  interview round (a new `stage_set` event) is a new episode.

**Alternative considered**: a `last_notified_at` timestamp + minimum re-notify
interval (snooze). Rejected — needs a magic interval to tune, and doesn't
distinguish "still the same silence" from "genuinely new silence after a reply,"
which the episode-key naturally does for free.

### D3: New `notification_settings` capability (renamed, shared), default flips for new rows only

`reminder_settings` → `notification_settings`, drop `default_delay_days`. Read by
both `internal/reminder` and `internal/nudge`. Absent-row default changes from
`enabled: false` to `enabled: true, channels: ['email']` — but this only changes
what "no row yet" means. The two rows already in production (one explicit opt-in,
one explicit opt-out) are left untouched; an explicit choice is never overwritten
by a change in what "never configured" defaults to.

### D4: `internal/nudge` gets its own `Notifier`/transports, not `reminder`'s

`internal/reminder/transports.go` hard-codes "you saved X and haven't applied yet"
copy. Branching that renderer by kind would blur `reminder`'s one job (saved-job
reminders). `nudge` gets its own minimal `Message`/`Notifier`/`Router` (same shape:
`Send(ctx, channel, dest, message) error`), its own email/Telegram transports with
per-kind copy, both linking to `/my/tracking/{id}`.

### D5: Own worker, `cmd/nudge`, not folded into `cmd/remind`

Mechanics differ: `cmd/nudge` is MATCH+DELIVER over live state (like `cmd/notify`),
`cmd/remind` is pure claim-and-fire over pre-scheduled rows. Own systemd timer,
cadence 30–60 minutes (final value in tasks) — frequent enough that "gone silent"
and "entered interview" are noticed same-day, infrequent enough to stay a light
batch job.

### D6: Settings UI moves to a new `/my/notifications` account-nav section

`ReminderSettings.svelte` (simplified: toggle + channel picker, no delay picker)
moves off `/my/activity` onto its own page. New entry in `accountNav.ts`/
`accountNavIcons.ts`. `/my/searches` (the saved-search subscription list) stays a
separate page — this is a new page, not a merge — but its existing "connect
Telegram on your notifications page" hint is repointed at `/my/notifications`.

### D7: Per-job reminder control removed entirely, not simplified to on/off

`ReminderChip.svelte`, `reminder.Override`, the reschedule/cancel API, and the
`reminderOverrideRequest` wire type are deleted rather than kept as a bare toggle.
Confirmed by direct user decision: this control has never been exercised by anyone
but the developer account.

## Risks / Trade-offs

- **[Risk] Opt-out-by-default for new accounts means some candidates get email/
  Telegram they never asked for.** → Mitigation: channel choice still requires a
  usable destination (email always is; Telegram needs the bot linked, so it never
  silently starts on an unlinked account), and the toggle is one click away in
  `/my/notifications`. This is an explicit, confirmed product decision, not an
  oversight.
- **[Risk] First deploy scanning ALL history for `follow_up`/`interview_prep`
  candidates could detonate a backlog of nudges for long-stale applications.** →
  Mitigation: both MATCH queries are bounded to a recency window (e.g. 30 days for
  `last_activity_at`, 7 days for the `stage_set` event), so old history never
  enters the ledger.
- **[Risk] Removing per-job reminder control is a real behavior regression for the
  one production user who uses it.** → Mitigation: usage check confirmed neither
  production account has ever touched the per-job chip or the delay picker: this is
  removing unused surface, not a regression for an active user.

## Migration Plan

1. Migration: rename `reminder_settings` → `notification_settings`, drop
   `default_delay_days`; create `application_nudges`.
2. Ship `internal/reminder` changes (drop `Override`/`DelayDays`) and
   `internal/nudge` together — the settings rename is a hard dependency for both.
3. Ship SPA changes (delete `ReminderChip.svelte`, relocate settings to
   `/my/notifications`) in the same release as the backend changes — the deleted
   per-job endpoints and the removed `reminder` param on save mean the SPA and API
   must move together.
4. Enable the `cmd/nudge` systemd timer after the release is verified live (health
   check + a manual MATCH-only dry run against prod data, per the existing
   `cmd/notify`/`cmd/remind` "log and exit 0 if unconfigured" pattern for a safe
   first run).
5. Rollback: the migration only renames/drops a column and adds a new table — no
   data loss on rollback of the rename (reverse migration restores
   `reminder_settings`/`default_delay_days`); `application_nudges` can simply be
   dropped.

## Open Questions

- Exact `cmd/nudge` cadence (30 vs. 60 minutes) and the two MATCH recency windows —
  finalized in `tasks.md` as implementation parameters, not architectural decisions.
