## Why

`/insights` answers "which roles are hiring" one category at a time, but never the
question a visitor actually brings — *where* each role is hiring, and where that is
moving. A role × country grid answers it at a glance, and freehire is in a position to
do what a frozen sample cannot: show live counts and their 30-day direction.

That direction is wrong today. Every insights growth figure counts a posting as
"open 30 days ago" only if freehire had already SEEN it then (`created_at`), so a newly
onboarded board's whole backlog reads as market growth. Measured on prod 2026-10-30:
of 876,973 open healthcare postings first seen in the last 30 days, 387,324 were
published by the employer BEFORE that window and 90,238 carry no publication date —
and the live `/insights/roles/healthcare` page states "Intern +15,074" against 20,531
open. A grid of growth arrows built on that measure would publish our onboarding
schedule as labour-market news, so the measure is fixed first.

## What Changes

- **Growth measures existence, not discovery.** Every insights rollup counts a posting
  as open at the window start from the EARLIER of its publication date and its
  first-seen date (`LEAST(posted_at, created_at)`), not from `created_at` alone. An old
  posting from a newly added board then sits in both windows and contributes no growth.
  Changes the published `growth` of `/api/v1/insights/roles`, `/api/v1/insights/skills`
  and `/api/v1/insights/companies` (`open_prev_30d`, `growth_30d`) — values, not shape.
  The daily hiring-velocity series counts first-seen days by design and is out of
  scope (see design.md, Open Questions).
- **New role × country rollup** covering every categorized canonical posting, not only
  the ones with a resolved seniority (the existing role rollup requires one, which
  leaves ~1.15M of ~4.4M categorized postings).
- **New endpoint** `GET /api/v1/insights/role-countries` returning the grid's cells
  (category × country, plus the all-countries column) with open count and prior count.
- **New grid on `/insights`**, above the category cards: roles as rows, the largest
  countries as columns, a switch between open counts and 30-day growth, every cell a
  link into `/jobs` for that role and country.

## Capabilities

### New Capabilities

- `insights-role-country-matrix`: the role × country grid — its rollup, its endpoint,
  and how the `/insights` page renders, colours and links it.

### Modified Capabilities

- `market-insights`: the "Precomputed insights rollups" requirement changes how
  open-as-of-a-date is derived (earliest of publication and first-seen date, not
  first-seen alone), which changes every published growth figure.

## Impact

- `internal/platform/db/queries/insights.sql` (growth predicate in every rebuild; new
  rebuild + read query), a new migration for the rollup table, regenerated
  `internal/platform/db`.
- `cmd/rollup-stats` (one more rebuild inside the existing transaction).
- `internal/api/handler/insights.go` + route registration (new handler).
- `web/src/routes/insights/` (+ a new component under `web/src/lib/components/`).
- No Meilisearch settings, no reindex, no LLM spend. The insights API is not in
  `web/static/openapi.yaml`, so the API-breaking check is unaffected.
- Growth figures on the live insights pages drop sharply on the first rollup after
  deploy — that is the fix, not a regression.
