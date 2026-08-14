# Notification frequency, quiet hours, and a profile timezone

## Problem

Every notification today delivers "as it happens" — `cmd/notify`/`cmd/remind`/
`cmd/nudge` each run every few minutes and send whatever they find, with no
regard for the time of day. There is no way to batch saved-search alerts into
a daily digest, and no way to stop anything from arriving at 3am.

## Goal

- A per-account choice for saved-search alerts: deliver instantly (today's
  behavior), or once a day at a chosen time.
- A per-account quiet-hours window that defers *every* notification kind
  (search alerts, saved-job reminders, follow-up/interview-prep/job-closed
  nudges) until it ends — nothing is dropped, only delayed to the next pass.
- A timezone field on the account so "9am" and "quiet after 10pm" mean the
  user's own 9am/10pm, not the server's.

Out of scope: per-saved-search frequency (one account-wide setting governs
all searches); merging multiple searches' daily digests into one message
(each saved search keeps sending its own, just delayed); quiet hours
suppressing the daily digest itself (a chosen digest time is not something
quiet hours should second-guess).

## Storage

- `users.timezone text` (nullable — absent means UTC). Edited on
  `/my/profile`'s Settings tab, alongside the existing `ProfileForm` fields.
  IANA name (e.g. `Europe/Moscow`), validated server-side against Go's
  `time.LoadLocation`.
- `notification_settings` gains three nullable columns, all specific to this
  feature and irrelevant to any other reader of that table:
  - `digest_frequency text NOT NULL DEFAULT 'instant'` (`instant` | `daily`)
  - `digest_time time` — meaningful only when `digest_frequency = 'daily'`;
    the UI always supplies a value (defaulting to 09:00) when switching to
    daily, so the two are never inconsistent in practice, but the column
    stays nullable rather than adding a CHECK that couples them.
  - `quiet_hours_start time`, `quiet_hours_end time` — both null means quiet
    hours are off (the default, preserving today's behavior for every
    existing account). Set together or not at all (UI-enforced, same pattern
    as the digest gating: the client never sends one without the other).

## Decisions

- **Frequency gates only `internal/notify`'s DELIVER phase.** Reminders and
  nudges are one-shot, trigger-bound events ("you saved this 3 days ago",
  "this stage just moved to interview") — batching them into a digest would
  change what they mean, not just when they arrive. Only the subscription
  digest engine reads `digest_frequency`/`digest_time`.
- **Daily mode is a narrowed claim window, not a new worker.** `cmd/notify`
  keeps its existing 5-minute cadence. For a `daily`-mode subscription, the
  DELIVER claim query additionally requires "now, in the account's timezone,
  falls within `[digest_time, digest_time+5min)`" — the same width as the
  cron interval, so exactly one pass per day satisfies it. Matches keep
  accumulating unclaimed in `subscription_matches` (already the ledger's
  job) until that window arrives; nothing new to build for the "waiting"
  half of this feature. If a pass is skipped (deploy, outage), that day's
  digest is skipped too and resumes on the next day's window — documented as
  an explicit trade-off, consistent with this codebase's existing "no retry
  queue" posture for the other engines.
- **Quiet hours is a claim-query predicate, duplicated across three
  engines, matching this codebase's established pattern.** `internal/notify`,
  `internal/reminder`, `internal/nudge` already independently implement
  "channel not configured → soft-skip" three times per
  `docs/agents/notifications.md`'s own documented stance ("adding a channel
  is... a wire-up in all three"); a delivery-time gate is the same shape of
  change. Each DELIVER claim query adds "now, in the account's timezone, is
  NOT within `[quiet_hours_start, quiet_hours_end)`" (handling the
  overnight-wraparound case where start > end, e.g. 22:00–08:00). A row that
  fails the check is simply not claimed this pass — the next pass re-evaluates
  it, same as any other unclaimed row.
- **Unset timezone defaults to UTC**, both for the quiet-hours check and for
  interpreting `digest_time` — an account that never visits its profile page
  keeps getting instant delivery (frequency defaults to `instant`, quiet
  hours default to off) regardless, so the UTC fallback only matters once
  someone opts into daily/quiet-hours without setting a timezone, which the
  UI should discourage (see below) but not hard-block.

## UI

- **Profile:** a timezone field on `/my/profile`'s Settings tab (`ProfileForm.svelte`), likely a searchable select over the IANA list rather than free text.
- **Notification settings:** a new block on `/my/notifications/settings` (`ReminderSettings.svelte` or a sibling component), below the existing channel picker:
  - Frequency: instant / daily radio or segmented control; daily reveals a time picker.
  - Quiet hours: an optional start/end time pair, off by default.
  - If the account has no timezone set and the user turns on daily or quiet hours, surface a hint pointing at `/my/profile` rather than silently assuming UTC.

## Risks / Trade-offs

- [Risk] A user who sets `digest_time` inside their own quiet-hours window
  gets a digest that (by design) ignores quiet hours — could read as a bug.
  → Mitigation: the settings UI shows both controls in the same panel, and
  the digest-time picker's helper text says explicitly that quiet hours
  don't apply to it.
- [Risk] Clock-change (DST) days shift `digest_time`'s UTC instant by an
  hour. → Mitigation: this is inherent to IANA-timezone-based scheduling and
  matches user expectation (9am stays 9am local, not 9am UTC); not treated
  as a defect.
- [Risk] Three near-identical quiet-hours predicates (one per engine) can
  drift out of sync if one is edited and the others aren't. → Mitigation:
  same posture this codebase already takes for the channel-registration
  duplication — accepted now, revisit if a future change makes the
  duplication cost look different (per `docs/agents/notifications.md`).

## Migration Plan

One additive migration: `users.timezone`, three new `notification_settings`
columns, all nullable/defaulted — no backfill, no existing-row rewrite, safe
to deploy ahead of the code that reads them.
