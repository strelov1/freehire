## 1. Measure before building

- [ ] 1.1 On prod, measure the share of OPEN `is_tech` postings with a non-empty
      `seniority`, and the same for a non-empty `category`. Record both numbers in this
      file. If seniority coverage is low, the page is gated on it (design risk 3) even
      though the rollup still ships.
- [ ] 1.2 `EXPLAIN (ANALYZE, BUFFERS)` the proposed aggregate
      (`FROM jobs, unnest(skills) … GROUP BY category, seniority, skill`) against prod and
      compare its buffers to the existing `RebuildInsightsRoleStatsByCountry`. Narrowing
      rows can widen I/O — judge on buffers, not on rows.

## 2. Rollup table and query

- [ ] 2.1 Add the migration creating `insights_role_skill_stats (category, seniority,
      skill, open_count, PRIMARY KEY (category, seniority, skill))`, with an index serving
      the ranked read `(category, seniority, open_count DESC)`. Comment why there is no
      country column, pointing at the sibling table's own rule.
- [ ] 2.2 `pnpm check:sql` passes on the new migration file.
- [ ] 2.3 Add `DeleteAllInsightsRoleSkillStats` and `RebuildInsightsRoleSkillStats` to
      `internal/platform/db/queries/insights.sql`, taking the sample floor as
      `@min_sample` exactly as the sibling rollups do. Run `make sqlc`.

## 3. Worker

- [ ] 3.1 In `cmd/rollup-stats`, run the delete + rebuild inside the SAME transaction as
      the existing insights rollups, so a reader never sees a partial rebuild.
- [ ] 3.2 Read the skill sample floor through the strict `worker.EnvInt` reader, so a
      set-but-unparseable value fails the run with the value named rather than silently
      taking a default.
- [ ] 3.3 Integration test: seeded jobs across two roles produce per-role distributions,
      a skill below the floor is omitted, closed postings do not contribute, and a rerun
      is idempotent.

## 4. API

- [ ] 4.1 Add `ListInsightsRoleSkills` to `internal/platform/db/queries/insights.sql`
      (ranked by `open_count DESC` within one role), regenerate with `make sqlc`.
- [ ] 4.2 Teach `InsightsRoles` the `seniority` parameter: validate against
      `vocab.SeniorityValues`, `400` on an unknown value, `400` when `seniority` is given
      without `category`.
- [ ] 4.3 Add `seniority` to this endpoint's read-parameter vocabulary so it never lands
      in `meta.ignored_params`. Assert this in a test — a dropped filter here widens the
      answer to every seniority.
- [ ] 4.4 When a single role is named, attach its `skills` array (`skill`, `open_count`,
      `share`). `share` is relative to the ROLE's open count, not the catalogue's.
- [ ] 4.5 When `country` is supplied alongside a single role, scope `open_count`/`growth`
      to the country, keep the distribution country-agnostic, and say so in `meta`.
- [ ] 4.6 Handler tests for every scenario in `specs/market-insights/spec.md`, including
      the empty-`skills` case for a role below the floor (200, never 404).

## 5. Signed-in coverage

- [ ] 5.1 When the request carries a session, read `userprofile.skills` and compute
      coverage with `jobmatch.Compute` over the role's ranked skills. Do not write a
      second matcher.
- [ ] 5.2 Report per skill: held / adjacent (naming the skill matched through, via
      `skilladjacency`) / missing, plus a held-out-of-total count.
- [ ] 5.3 A signed-in caller with no profile skills gets a coverage section reporting zero
      held — never an absent section, which a client cannot distinguish from signed-out.
- [ ] 5.4 Set `Cache-Control: private` whenever a coverage section is present, and assert
      it in a test. The sibling insights routes set a shared-cache header by default.
- [ ] 5.5 Handler tests for every scenario in `specs/role-skill-coverage/spec.md`,
      including the anonymous path returning 200 with no coverage.

## 6. Web

- [ ] 6.1 Add the client method in `web/src/lib/api.ts` for the single-role read.
- [ ] 6.2 Add `web/src/routes/insights/roles/[category]/[seniority]/+page.server.ts`,
      gated by the same `loadInsightsGate` / `isCovered` read the sibling pages use, and
      404 for a pair that does not clear the gate.
- [ ] 6.3 Build `+page.svelte`: role header with open count and growth, the ranked skill
      list, and a link into the filtered jobs search for that role. Follow the existing
      insights pages' layout rather than inventing one.
- [ ] 6.4 Label the column "mentioned in", never "required by". This wording is normative
      in the spec.
- [ ] 6.5 Overlay the coverage for a signed-in visitor: held / adjacent (showing the
      neighbour) / missing, plus the held-out-of-total line. Anonymous visitors see the
      aggregate alone with no empty column.
- [ ] 6.6 Link each seniority row on `/insights/roles/[category]` to its new leaf page.
- [ ] 6.7 Extend `insightsPaths` and `sitemap-insights.xml` to list the (category,
      seniority) leaves that clear the gate — and only those, matching what the route
      serves.
- [ ] 6.8 `pnpm check:dead` passes (knip gates unused exports, types included).

## 7. Verify and ship

- [ ] 7.1 `gofmt -l .` prints nothing; `go vet ./...`; `go test ./...`;
      `go vet -tags=integration ./...`.
- [ ] 7.2 `pnpm check:links` — this change adds no new doc links, but renaming a target
      breaks them.
- [ ] 7.3 Deploy migration + worker. Let one nightly `cmd/rollup-stats` run fill the table.
- [ ] 7.4 Read that run: rows produced, roles clearing the floor, seniority coverage. SET
      the floor from these numbers and record them here. Do not guess the floor earlier.
- [ ] 7.5 Deploy the endpoint (additive — without `seniority` the response is unchanged).
- [ ] 7.6 Deploy the page and sitemap entries once 7.4's numbers justify publishing.
- [ ] 7.7 No reindex. Nothing here touches Meilisearch, `content_hash`, or a facet.
