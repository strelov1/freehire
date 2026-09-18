## Why

The catalogue already answers "which roles are hiring" (`insights_role_stats`, category ×
seniority) and "which skills are in demand" (`insights_skill_stats`, skill scoped by
category OR country). It cannot answer the question a candidate actually asks: **"what does
*this* role want, and how far am I from it?"** Skill demand is never crossed with
seniority, so `Senior Backend` and `Junior Backend` are indistinguishable in the skill
rollup, and nothing anywhere compares a role's skill demand against the signed-in user's
own profile.

The gap is cheap to close: the grouping key, the rollup worker, the atomic
delete-and-reinsert, the sample floor, the insights coverage gate, the sitemap shard and
the per-category landing page all exist. What is missing is one rollup table, one SQL
aggregate, one endpoint field and one leaf page.

## What Changes

- New rollup table `insights_role_skill_stats (category, seniority, skill, open_count)` —
  the skill distribution *within* one role. Deliberately not scoped by country: crossing a
  third axis multiplies rows for a slice nobody has asked for, and the existing
  `insights_skill_stats` comment already records that category and country are not crossed
  in one row for the same reason.
- `cmd/rollup-stats` gains one aggregate that fills it, inside the same transaction and
  under the same `@min_sample` floor the sibling rollups already take.
- `GET /api/v1/insights/roles` gains an optional `seniority` parameter. When a single role
  is named (both `category` and `seniority`), the response carries that role's ranked skill
  distribution alongside its open-count and growth.
- When the request carries a session cookie, the same response additionally carries the
  caller's coverage of that role's skills — held, adjacent, and missing — computed from
  `userprofile.skills` through the existing `internal/candidate/jobmatch.Compute` and
  `internal/dict/skilladjacency`. Anonymous callers get the aggregate alone; the endpoint
  stays readable without a session.
- New page `/insights/roles/[category]/[seniority]`, the leaf under the existing
  `/insights/roles/[category]` list. It renders the role's skill demand, and for a
  signed-in visitor overlays their own coverage.
- `sitemap-insights.xml` lists the new leaf pages for every (category, seniority) pair that
  clears the coverage gate.

Not in scope, and deliberately: psychometric/cognitive matching (it cannot be validated
against hiring outcomes we do not have), non-IT roles (the `is_tech` gate and the category
vocabulary are built for IT), and an employer-side requirements autofill.

## Capabilities

### New Capabilities

- `role-skill-coverage`: the signed-in overlay on a role's skill demand — which of the
  role's ranked skills the caller holds, which they hold a neighbour of, and which they are
  missing. Separate from `market-insights` because that capability is defined as public,
  unauthenticated and aggregate-only, and this reads one caller's own profile.

### Modified Capabilities

- `market-insights`: adds per-role skill demand — the skill distribution within one
  (category, seniority) role, reachable by naming a single role on the existing
  `GET /api/v1/insights/roles` endpoint.

## Impact

- **Migrations**: one new table (`insights_role_skill_stats`). No change to an applied file.
- **SQL layer**: new queries in `internal/platform/db/queries/insights.sql`; `make sqlc`.
- **Workers**: `cmd/rollup-stats` — one additional aggregate. No new binary, so no new
  systemd unit and no build-list edit (the trap `close-chronic-boards` fell into).
- **API**: `internal/api/handler/insights.go` — `InsightsRoles` gains the `seniority`
  parameter and the two new response sections. `UnknownParams` vocabulary for this endpoint
  must learn `seniority`, or the new filter would be silently dropped and reported.
- **Reused unchanged**: `internal/candidate/jobmatch`, `internal/dict/skilladjacency`,
  `internal/identity/userprofile`.
- **Web**: new route `web/src/routes/insights/roles/[category]/[seniority]/`; `insightsPaths`
  and `sitemap-insights.xml` extended; `web/src/lib/api.ts` client method.
- **No reindex.** Nothing here touches Meilisearch, `content_hash`, or any facet.
- **Rollback** is dropping the page and leaving the table unread; the rollup is additive and
  its absence costs the existing insights pages nothing.
