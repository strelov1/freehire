## Context

`cmd/rollup-stats` already recomputes `insights_skill_stats` intra-day (every
few hours) as a full delete-and-reinsert inside one transaction — it is
explicitly documented as "a pure function of current `jobs` state... no
historical snapshot table is needed" (`migrations/0022_insights_rollups.sql`).
That is correct for the existing two-point comparison (`open_count` vs.
`open_count_prev`, a fixed 30-day-back window), but it means there is nowhere
to read "the last 6 months of demand for Go" from — the moment a run
recomputes the table, the previous state is gone.

This change adds the one thing that rebuild-in-place model cannot provide:
a real multi-point history. It does so as a narrow, additive companion table
rather than changing how `insights_skill_stats` itself is computed.

## Goals / Non-Goals

**Goals:**
- Retain roughly 6 months of weekly skill-demand snapshots, keyed by
  canonical skill.
- Let a signed-in user read their own profile skills' history + trend in one
  call.
- Reuse the skill open-counts `cmd/rollup-stats` already computes each run —
  no second full scan of `jobs`.

**Non-Goals:**
- Category- or country-scoped history (the existing `insights_skill_stats`
  table already carries `(skill, category, '')` and `(skill, '', country)`
  rows in addition to the global `(skill, '', '')` bucket; this change only
  snapshots the global bucket).
- Salary trend, "skills to learn" recommendations, email digest (proposal's
  explicit non-goals — future changes, not this one).
- Backfilling history before this change ships — the series starts empty and
  grows one week at a time; a skill card shows a short/flat series until it
  has accumulated a few weeks.

## Decisions

**A companion table, not a new pattern.** `insights_skill_history(skill,
week_start, open_count)`, PK `(skill, week_start)`. Populated by one
additional `INSERT ... ON CONFLICT (skill, week_start) DO NOTHING` statement
appended to `rebuildInsights` in `cmd/rollup-stats/main.go`, selecting from
`insights_skill_stats WHERE category = '' AND country = ''` — i.e. reading the
rollup this same transaction just rebuilt, not re-aggregating `jobs`.
`week_start` is `date_trunc('week', now())::date` (ISO week, Monday). The
`ON CONFLICT DO NOTHING` makes the insert naturally idempotent across the
several-times-a-day cron cadence: every run this week after the first is a
no-op INSERT, not a scheduling problem to solve separately.
*Alternative considered*: a dedicated weekly cron worker. Rejected — it would
re-derive open-counts from `jobs` a second time for numbers `rollup-stats`
already has in the same transaction, and adds a second thing to schedule and
monitor for one INSERT statement's worth of work.

**Pruning inline, not a separate job.** The same rollup-stats run deletes
`insights_skill_history` rows older than the retention window (26 weeks) right
before/after the conditional insert, in the same transaction. No separate
prune worker.

**Read path is a plain join, not its own rollup.** `GET /me/market-pulse`
reads `userprofile.Service.Get(ctx, userID).Skills`, then one query against
`insights_skill_history` for those skills over the retention window, grouped
in Go into `{skill, open_count (latest week), change_pct, series: [{week_start,
open_count}]}`. This is cheap (bounded by the user's own skill count × ~26
rows) and needs no rollup of its own — the per-request cost is proportional to
one user's profile, not the catalogue.

**Empty-profile handling.** A user with no skills in their profile (new
signup, no CV parsed yet) gets `data: []` with `200`, not an error — the
frontend page shows an empty state pointing at CV upload, not a failure.

**Response envelope.** Matches project convention: `{"data": [...], "meta":
{...}}`; `meta` carries the resolved `week_start` used as "current".

## Risks / Trade-offs

- **[Risk]** A skill renamed/merged in the canonical dictionary
  (`internal/skilltag`) breaks continuity — old weeks used the old name, new
  weeks use the new one, so the series looks like it dropped to zero.
  → **Mitigation**: none in this change (dictionary renames are already rare
  and out-of-band); accepted as a known, pre-existing limitation shared with
  `insights_skill_stats` itself, not something to special-case here.
- **[Risk]** `insights_skill_stats` is fully cleared and reinserted each
  `rollup-stats` run; if that step fails after the delete but before the
  reinsert commits, the snapshot INSERT for `insights_skill_history` never
  runs (same transaction) — no partial/bad history row is possible, only a
  skipped week. → **Mitigation**: none needed; the next intra-day run within
  the same week retries via the same `ON CONFLICT DO NOTHING` path.
- **[Trade-off]** History starts empty at ship time — the first few weeks
  show a short series / no trend arrow. Accepted; called out explicitly in
  the proposal's non-goals rather than solved with a synthetic backfill.

## Migration Plan

Additive only — one new migration (`insights_skill_history` table + index +
retention delete), one modified worker (`cmd/rollup-stats`), one new handler,
one new frontend route. No existing table, endpoint, or response shape
changes. Deploy order follows the standard rule (migrate before code that
reads new schema); `cmd/rollup-stats`'s next scheduled run after deploy
performs the first snapshot insert. No rollback hazard beyond dropping the new
table/migration.

## Open Questions

None outstanding — retention (6 months), granularity (weekly), and skill
selection (unfiltered profile skills) were decided during brainstorming.
