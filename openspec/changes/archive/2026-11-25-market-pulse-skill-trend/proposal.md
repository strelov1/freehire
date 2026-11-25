## Why

Job seekers currently see only a snapshot of skill demand (`GET
/api/v1/insights/skills`, `open_count` vs. a single point 30 days back) and have
no reason to return to freehire between active searches. A personalized,
recurring "is my stack getting more or less in-demand" view — computed from
their own CV skills — gives them a standing reason to check back weekly and
differentiates freehire from listing-only competitors.

## What Changes

- Add a new append-only weekly snapshot table (`insights_skill_history`) that
  records, per canonical skill, the count of currently open jobs listing it —
  one row per ISO week, retained for roughly 6 months, then pruned.
- Extend the existing `cmd/rollup-stats` worker to write one snapshot row per
  skill per ISO week (idempotent — skips if this week's row already exists),
  reusing the skill open-counts it already computes for `insights_skill_stats`
  rather than re-scanning `jobs`.
- Add an authenticated `GET /api/v1/me/market-pulse` endpoint that reads the
  caller's own `userprofile.Profile.Skills`, joins them against the weekly
  history, and returns per-skill current `open_count`, percent change, and a
  sparkline series.
- Add a new SvelteKit page `/my/market-pulse` rendering one card per skill in
  the user's profile: current count, an up/down indicator, and a 6-month
  sparkline.
- Explicitly out of scope for this change: salary trend, "skills to learn"
  recommendations, and an email digest — noted as possible future follow-ups,
  not part of this change's tasks.

## Capabilities

### New Capabilities

- `market-pulse`: personalized, authenticated weekly skill-demand trend for
  the signed-in user's own profile skills — the history rollup, the read API,
  and the dashboard page.

### Modified Capabilities

(none — `market-insights`'s existing public aggregate endpoints and rollup
contract are unchanged; the new weekly history table and its writer are net-new
and owned by the `market-pulse` capability, even though the writer rides along
in the same `cmd/rollup-stats` process for efficiency.)

## Impact

- **DB**: one new migration adding `insights_skill_history` (+ index on
  `(skill, week_start)`) and a pruning condition for rows older than the
  retention window.
- **Go**: `cmd/rollup-stats/main.go` (new weekly-snapshot step), new sqlc
  queries, a new handler for `GET /api/v1/me/market-pulse` under
  `internal/handler/` (`RequireAuth`), reads `internal/userprofile.Profile.Skills`.
- **Web**: new route `web/src/routes/my/market-pulse/` and a fetch against the
  new endpoint; no changes to existing pages.
- **No breaking changes** — purely additive.
