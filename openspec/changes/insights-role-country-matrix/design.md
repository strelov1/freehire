## Context

`cmd/rollup-stats` rebuilds the `insights_*` tables in one transaction
(delete-and-reinsert), and `/api/v1/insights/*` reads them. The role rollup
(`insights_role_stats`) is keyed `(category, seniority, country)` and only counts
postings with BOTH a category and a seniority — on 2026-10-30 that was ~1.15M rows,
against ~4.4M categorized postings in search. Seniority is resolved for a minority of
postings, so a grid derived from that table would describe a quarter of the catalogue.

Every prior-window count in `insights.sql` uses the same predicate,
`created_at <= @prev_ts AND (closed_at IS NULL OR closed_at > @prev_ts)` — six
statements (seven occurrences): role global/by-country, skill
global/by-category/by-country, and company growth (twice, in its SELECT and its
non-zero filter). `created_at` is first-seen. The proposal's measurement (387k of 877k "new"
open healthcare postings published before the window) is what that predicate costs.

## Goals / Non-Goals

**Goals:**
- One honest growth rule, applied by every insights rollup.
- A role × country grid covering every categorized canonical posting, live, with a
  30-day direction, readable without JavaScript.

**Non-Goals:**
- An industry axis (deferred: company industry covers ~41% of postings, see the
  conversation that produced this change).
- Seniority or work-mode axes (resolved for ~18% / ~22% of postings).
- Re-basing the existing role/skill rollups on canonical postings (they count
  duplicates today; changing it moves every published count — a separate decision).
- The daily velocity series (see Open Questions).
- New per-cell landing pages — the `(category, country)` role landings already exist;
  the grid links to them (Decision 4).

## Decisions

### 1. Existence = `LEAST(posted_at, created_at)`

Postgres' `LEAST` ignores NULLs, so one expression covers all three cases: a
publication date earlier than first-seen wins (the onboarding backlog stops reading
as growth), a missing publication date falls back to first-seen, and a publication
date LATER than first-seen (sources that re-stamp on repost) cannot push existence
forward. The predicate becomes
`LEAST(posted_at, created_at) <= @prev_ts AND (closed_at IS NULL OR closed_at > @prev_ts)`.

*Alternatives:* `posted_at` alone — loses the ~10% with no date and trusts re-stamped
dates. Like-for-like boards ("only boards that existed at both ends") — the honest
retail-style answer, but `boards.created_at` is 2026-09-03 for nearly every board (the
table's migration date), so it cannot tell when a board was onboarded.

*Residual bias:* a posting published and closed before freehire ever saw it is in
neither window. For a newly onboarded board that under-counts the prior window, so
growth stays slightly inflated — far less than today, and stated on the page.

The predicate is written inline (seven occurrences today, nine after this change). A
SQL function would centralise it but would hide it from sqlc reviewers reading one
query; a test instead asserts every `open_count_prev` expression in `insights.sql`
uses `LEAST(posted_at, created_at)` — the one-reader-rule trap in reverse.

### 2. A separate `insights_role_country_stats` table

`(category, country, open_count, open_count_prev)`, PK `(category, country)`,
`country = ''` for all countries — the same sentinel the sibling tables use.

*Alternative:* add `seniority = ''` "all seniorities" rows to `insights_role_stats`.
Rejected: `ListInsightsRoles` ranks rows as roles, so every existing reader would need
a new `seniority <> ''` guard, and the two tables would count different populations
(raw vs canonical) under one name.

Two rebuild statements, like the role rollup: a global pass (`GROUP BY category`, so a
multi-country posting counts once) and a by-country pass over `unnest(countries)`.
Population: `duplicate_of IS NULL AND NOT is_private AND category <> ''`. That is what
`/jobs` admits apart from the description check, which would de-TOAST every row and is
deliberately skipped — so a cell approximates, not equals, the count its link shows.

### 3. The endpoint picks the columns

`GET /api/v1/insights/role-countries?countries=N` (default 10, max 25). Column order is
`SUM(open_count)` per country over the by-country rows. Returning the cells for those
columns plus the all-countries bucket keeps the payload ~50 × 11 rows and lets the API
— not each client — own "which countries matter". Raw `open_count` and
`open_count_prev` go on the wire; the percentage and its floor are presentation and
stay in the page, so a future consumer can choose its own floor.

### 4. Page: one component, SSR

`web/src/routes/insights/+page.server.ts` fetches the endpoint beside the existing
gate, under the same `s-maxage=3600`. A new `RoleCountryGrid.svelte` renders a real
`<table>` (`th scope`), the first column sticky so a phone scrolls the countries
sideways. The open/growth switch and "show all" are client state over data already on
the page — no second request. Labels come from the existing category and country
display helpers.

- **Open view shading:** `open_count / column total`, single-hue sequential scale,
  scaled per view to the largest share present.
- **Growth view:** signed percentage with ▲/▼, diverging scale around zero, clamped at
  ±100% so one outlier does not wash out the rest; floor `open_count_prev < 50` → "—".
- **Links:** the role landings already exist — `/roles/<category>` (one category's
  country table) and `/roles/<category>/<country>` (the pair page, published for
  ~1,471 pairs with at least `MIN_PAIR_OPEN` = 50 open postings, carrying salary,
  skills and jobs). They are exactly the per-cell pages a grid wants to point at, so a
  cell links to its pair page when the category has a landing (`categoryLandingLink`,
  `countrySlug`) and the cell holds ≥ 2 × `MIN_PAIR_OPEN`; otherwise to
  `/jobs?category=<c>&countries=<cc>`. The 2× margin: the pair page gates on the live
  Meilisearch facet count, the grid reads the Postgres rollup (no description check,
  up to one run stale), and `categoryLandingLink`'s own comment records why linking a
  pair unchecked from an indexed page is worse than not linking. The all-countries
  cell → `/roles/<category>`; row heading → `/insights/roles/<c>`; column heading →
  `/jobs?countries=<cc>`. Reusing `MIN_PAIR_OPEN` rather than a second constant keeps
  the margin tied to the floor it protects.

  *Alternative:* read the grid from Meilisearch facets (one call per column), so
  counts equal the gate exactly. Rejected: the facet index has no history, so the
  growth view would still need the rollup, and a grid mixing two sources shows two
  different numbers for one cell across its two views.
- Colours from design-system tokens; the visual pass uses the `impeccable` skill at
  implementation time.

## Risks / Trade-offs

- [Every published growth figure drops after deploy] → expected; the proposal says
  so, and the change description in the PR states the before/after for healthcare.
- [Grid's all-countries column disagrees with the category cards below it (canonical
  vs raw, all vs seniority-resolved)] → the cards say "open roles" from the role
  rollup; reconciling them is the separate re-basing decision listed in Non-Goals.
  Recorded so a reviewer does not "fix" it silently.
- [Cell count ≠ the count on the linked search] → approximate by design (no
  description check, rollup lag up to one run). Not presented as exact.
- [Rollup run gets longer] → one more `unnest(countries)` pass, the same shape as the
  role-by-country pass whose cost is already measured in production.
- [`posted_at` in the future (bad source data)] → `LEAST` takes `created_at`, so the
  posting still exists from first-seen.

## Migration Plan

1. Migration adds `insights_role_country_stats` (empty table, no backfill).
2. Deploy: `release.sh` migrates, the new binary rebuilds all rollups on its next
   `rollup-stats` run. Until that run the grid's endpoint returns an empty `data`, and
   the page hides the grid when `data` is empty rather than rendering a blank table.
3. Rollback: revert the binary; the table is inert. The growth change has no state —
   reverting restores the old figures on the next run.

## Open Questions

- The velocity series (`RebuildInsightsVelocityDaily`) plots "added" on the
  first-seen day, so an onboarding wave shows as a hiring spike. It answers "what did
  freehire add", which is a legitimate question with a misleading label; whether to
  relabel or re-base it is left to a separate change.
