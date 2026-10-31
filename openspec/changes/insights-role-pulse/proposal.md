## Why

`/insights/roles/<category>` says how many postings a category has and how each
seniority moved, but not what a job seeker actually weighs before choosing a direction:
how crowded the market is, how fast vacancies get filled, and whether new ones keep
arriving. freehire already holds the evidence for all three — page views per posting,
the close reason of every posting, and when each one was published — and publishes none
of it per role.

The same page also understates its own headline. "There are N open <Category> roles"
sums the seniority rows of `insights_role_stats`, which only counts postings with a
resolved seniority: about a quarter of the categorized catalogue (2026-10-30). The
all-seniority figure exists since `insights_role_country_stats` (country `''`).

## What Changes

- **New rollup `insights_role_pulse`**, one row per category, rebuilt by
  `cmd/rollup-stats` inside its existing transaction:
  - **competition** — page views (`job_daily_views.page_uniques`, never `uniques`) of
    the category's postings and the posting-days they were visible on freehire, for
    the last 30 days of view data and the 30 before;
  - **time to close** — the count and the median lifetime of the category's postings
    that closed for a reason that says something about the posting (not about our
    crawler), in the last 30 days and the 30 before;
  - **inflow** — postings the employer published in the last 30 days and the 30
    before, by existence date `LEAST(posted_at, created_at)`.
- **New endpoint** `GET /api/v1/insights/role-pulse?category=<c>`: the three raw
  figure pairs plus the category's all-seniority open count. Aggregate-only, additive.
- **`/insights/roles/<category>`** shows the three figures with their 30-day
  direction, each hidden below a sample floor, and its intro counts every open
  posting of the category instead of the seniority-resolved subset.

## Capabilities

### New Capabilities

- `insights-role-pulse`: the per-category market pulse — its rollup, its endpoint, and
  how the roles page renders it and its corrected open count.

### Modified Capabilities

(none — the roles ranking endpoint and its rollup are unchanged)

## Impact

- New migration (`insights_role_pulse`), `internal/platform/db/queries/insights.sql`
  (rebuild + two reads), regenerated `internal/platform/db`.
- `cmd/rollup-stats`: one more delete-and-rebuild in the same transaction. Measured
  2026-10-30, the query took 7m28s on prod as a standalone SELECT under load.
- `internal/api/handler/insights.go` + route; `web/src/lib/api.ts`, a new pure helper
  `web/src/lib/rolePulse.ts`, the roles page and `rolesIntro`.
- No Meilisearch settings, no reindex, no LLM spend. The insights API is not in
  `web/static/openapi.yaml`.
