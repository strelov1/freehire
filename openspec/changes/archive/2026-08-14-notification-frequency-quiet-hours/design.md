## Context

Full write-up of motivation/scope decisions:
`docs/superpowers/specs/2026-08-14-notification-frequency-quiet-hours-design.md`.
This document verifies and finalizes the mechanics against the actual code in
`internal/notify`, `internal/reminder`, `internal/nudge`.

All three engines already share one shape: a bulk `ClaimXXX` query leases a
batch of due rows (pure time+lease predicate, `FOR UPDATE ... SKIP LOCKED`),
then a per-item `fire`/`deliverOne` loads a `GetXForDelivery` row (job fields,
live user destinations, live re-check flags) and either delivers, cancels, or
soft-skips via `r.release(ctx, id)` — which drops the claim so the item is
retried on a later pass, no failed attempt burned. This existing
soft-skip/release path is a channel-not-configured check today
(`internal/notify/deliver.go`'s `recipient(info)`,
`internal/reminder/engine.go`/`internal/nudge/nudge.go`'s "no channel had a
usable destination"); quiet hours becomes one more condition on the same
path, at the same call sites, using the same mechanism.

## Goals / Non-Goals

**Goals:**
- `instant`/`daily` frequency for saved-search digests, gated at
  `internal/notify`'s existing per-item delivery decision.
- A quiet-hours window that defers delivery (never drops it) across all
  three engines' existing per-item delivery decisions.
- A `users.timezone` field, defaulting to UTC when unset.

**Non-Goals:**
- Any change to the bulk `ClaimXXX` leasing queries or their `SKIP LOCKED`
  concurrency guarantee.
- Any change to reminder/nudge's own `enabled`/`channels` gate
  (`notification_settings.enabled`) — quiet hours/frequency are additive
  conditions alongside it, not a replacement.
- A cron-cadence-independent digest mechanism was considered and rejected in
  favor of an even more robust one — see Decisions.

## Decisions

- **Quiet hours is a per-item delivery-decision check, not a claim-query
  predicate.** Verified against the actual `fire`/`deliverOne` code in all
  three engines: each already loads a `GetXForDelivery` row and has an
  established "not deliverable right now → `r.release()`, soft-skip, retry
  later" branch. Quiet hours slots in as one more branch there, calling the
  same `release` function each engine already has
  (`ReleaseMatchClaim`/`ReleaseReminderClaim`/`ReleaseNudgeClaim`). This
  needs `u.timezone` and `ns.quiet_hours_start`/`ns.quiet_hours_end` added to
  the three `GetXForDelivery` queries: `GetNudgeForDelivery` already `LEFT
  JOIN`s `notification_settings`, just gains columns;
  `GetSubscriptionForDelivery` and `GetReminderForDelivery` gain the join.
  A shared Go helper (new leaf package `internal/deliverywindow`, imported by
  all three engines, importing none of them — mirrors the existing rule that
  no engine imports another) computes `InQuietHours(now time.Time, tz
  string, start, end *time.Time) bool`, handling the overnight-wraparound
  case (`start > end`, e.g. 22:00–08:00) in Go rather than SQL.
- **Daily digest tracks its own "last sent" instant, not a cron-relative time
  window.** The design doc's original sketch ("deliver only inside a
  5-minute window matching the cron cadence") was rejected once checked
  against reality: the cron interval lives in the *ops* repo, not this one,
  is not read by the Go code, and could change independently — a
  window-width baked into Go that silently drifts from the actual cron
  cadence would intermittently skip or double-fire a day's digest. Instead,
  `subscriptions` gains `last_digest_sent_at timestamptz` (nullable). A
  `daily`-mode subscription is due when, in the account's timezone, the
  local time has passed `digest_time` AND (`last_digest_sent_at` is null OR
  its local calendar date precedes today's local calendar date). This fires
  on the *first* pass after `digest_time` each day regardless of cron
  granularity, and is naturally idempotent against any number of passes
  within the same day. `internal/deliverywindow` also owns this check:
  `DigestDue(now time.Time, tz string, digestTime *time.Time, lastSentAt
  *time.Time) bool`. On a successful daily delivery, `deliverOne` stamps
  `last_digest_sent_at = now()` on the subscription (folded into the
  existing `MarkMatchesNotified` update, or a sibling one-line query run
  right after it).
- **`instant`-mode subscriptions are unaffected by `DigestDue`** — that
  check only runs when `digest_frequency = 'daily'`; `instant` keeps
  today's exact behavior (deliver as soon as claimed, no additional gate
  beyond quiet hours).
- **Quiet hours does not gate a `daily`-mode digest.** Per the proposal: a
  chosen digest time is itself the user's preference. `deliverOne` checks
  `InQuietHours` only when `digest_frequency = 'instant'`.
- **Absent `notification_settings` row / absent `users.timezone` behave as
  today.** `LEFT JOIN` means a `NULL` `quiet_hours_start` (off) and `NULL`
  `digest_frequency` (read as `instant` in Go, since the column itself
  defaults `NOT NULL DEFAULT 'instant'` once a row exists, but a row may not
  exist at all — same absent-row-means-default posture as the existing
  `enabled`/`channels` gate). `NULL` `users.timezone` reads as `UTC`.

## Risks / Trade-offs

- [Risk] Three call sites each gain a quiet-hours branch — the accepted,
  existing duplication pattern in this codebase (per
  `docs/agents/notifications.md`), not a new one introduced here.
- [Risk] `last_digest_sent_at`'s local-date comparison is itself
  timezone-dependent — a user who changes their timezone at the boundary of
  a digest day could see one day skipped or doubled. Accepted as a rare
  edge case, same posture as the DST risk already noted in the referenced
  design doc.
- [Risk] Adding a join + two-to-four columns to three hot per-item delivery
  queries is a minor read-cost increase. All three already join `users`;
  the added `notification_settings` join is on an indexed `user_id` (the
  table's existing unique key), so the cost is one extra indexed lookup per
  item, not a scan.

## Migration Plan

One additive migration: `users.timezone text`; `notification_settings`
gains `digest_frequency text NOT NULL DEFAULT 'instant'`, `digest_time
time`, `quiet_hours_start time`, `quiet_hours_end time`;
`subscriptions` gains `last_digest_sent_at timestamptz`. All
nullable/defaulted — no backfill, safe to deploy ahead of the code that
reads them.
