## 1. Measure before building

- [x] 1.1 On prod, measure the share of OPEN `is_tech` postings with a non-empty
      `seniority`, and the same for a non-empty `category`. Record both numbers in this
      file. If seniority coverage is low, the page is gated on it (design risk 3) even
      though the rollup still ships.

      **Measured 2026-09-18, production `hire`:**

      | | rows | share of open `is_tech` |
      |---|---|---|
      | open postings, all | 5,978,300 | |
      | open, `is_tech` | 1,012,085 | 100% |
      | …with `category <> ''` | 998,688 | **98.7%** |
      | …with `seniority <> ''` | 394,610 | **39.0%** |
      | …with both | 392,020 | 38.7% |
      | …with both and `cardinality(skills) > 0` | 348,060 | 34.4% |

      **Risk 3 fired.** 61% of open tech postings state no seniority, and they are not a
      random 61% — they are the postings whose title names no level. Two artifact updates
      followed (see design.md and specs/market-insights/spec.md): the sample size becomes a
      served field with a normative wording rule, and the category-only distribution
      (98.7% coverage, already in `insights_skill_stats`) is rendered beside the role's as a
      backstop.

      **Second finding, not anticipated:** 11% of eligible postings carry no tagged skill
      (392,020 → 348,060), so the share's denominator had to be pinned. It is now the
      skill-bearing subset, served as `sample_size`.
- [ ] 1.2 `EXPLAIN (ANALYZE, BUFFERS)` the proposed aggregate
      (`FROM jobs, unnest(skills) … GROUP BY category, seniority, skill`) against prod and
      compare its buffers to the existing `RebuildInsightsRoleStatsByCountry`. Narrowing
      rows can widen I/O — judge on buffers, not on rows.

## 2. Rollup table and query

- [x] 2.1 Add the migration creating `insights_role_skill_stats (category, seniority,
      skill, open_count, PRIMARY KEY (category, seniority, skill))`, with an index serving
      the ranked read `(category, seniority, open_count DESC)`. Comment why there is no
      country column, pointing at the sibling table's own rule.
- [x] 2.2 In the same migration, add `insights_role_skill_sample (category, seniority,
      sample_size, PRIMARY KEY (category, seniority))` — the share's denominator, one row
      per role, at most 216 rows. A separate table rather than a column on
      `insights_role_stats` (which is country-keyed, so the figure would be meaningless on
      every non-'' row) and rather than a value repeated on every skill row (one fact
      written a dozen times). Comment the 11% measurement that forced it.
- [x] 2.3 `pnpm check:sql` passes on the new migration file. Both count columns carry a
      `-- squawk-ignore prefer-bigint-over-int` with the argument beside it: a per-role
      posting count sits four orders of magnitude below int's ceiling, and `integer` is
      what the rest of the insights family already serves as int32. Note the suppression
      syntax — the reason must follow a SECOND `--`, or squawk parses the prose as more
      rule names and reports each word as an unknown rule.
- [x] 2.4 Add `DeleteAllInsightsRoleSkillStats` and `RebuildInsightsRoleSkillStats` to
      `internal/platform/db/queries/insights.sql`, taking the sample floor as
      `@min_sample` exactly as the sibling rollups do. Run `make sqlc`.
- [x] 2.5 Add the sibling `DeleteAllInsightsRoleSkillSample` /
      `RebuildInsightsRoleSkillSample` — `count(*) FILTER (WHERE cardinality(skills) > 0)`
      per role. It takes NO `@min_sample`: the denominator must exist for every role whose
      skills were counted, and flooring it would make some shares undividable.

## 3. Worker

- [x] 3.1 In `cmd/rollup-stats`, run the delete + rebuild inside the SAME transaction as
      the existing insights rollups, so a reader never sees a partial rebuild.
- [x] 3.2 Read the skill sample floor through the strict `worker.EnvInt32` reader
      (`ROLE_SKILL_MIN_SAMPLE`, default 5 — a knob rather than a constant like its
      siblings precisely because 7.4 is what sets it), so a
      set-but-unparseable value fails the run with the value named rather than silently
      taking a default.
- [x] 3.3 Integration test: seeded jobs across two roles produce per-role distributions,
      a skill below the floor is omitted, closed postings do not contribute, and a rerun
      is idempotent.

## 4. API

- [x] 4.1 Add `ListInsightsRoleSkills` and `GetInsightsRoleSkillSample` to `internal/platform/db/queries/insights.sql`
      (ranked by `open_count DESC` within one role), regenerate with `make sqlc`.
- [x] 4.2 Teach `InsightsRoles` the `seniority` parameter: validate against
      `vocab.SeniorityValues`, `400` on an unknown value, `400` when `seniority` is given
      without `category`.
- [x] 4.3 Make `seniority` a READ parameter. Correction to how this task was written:
      `/insights/*` has no `meta.ignored_params` mechanism at all, so there is no
      vocabulary to add it to — and that is worse, not better. Until this change the
      endpoint silently ignored `?seniority=`, answering with EVERY seniority and with no
      way for the caller to know. The test asserts it now narrows.
- [x] 4.4 When a single role is named, attach its `skills` array (`skill`, `open_count`,
      `share`) and its `sample_size`. `share` divides by `sample_size` — the role's
      skill-bearing postings — never by `open_count` and never by the catalogue. Test the
      worked example from the spec (710/900, not 710/1000).
- [x] 4.5 When `country` is supplied alongside a single role, scope `open_count`/`growth`
      to the country, keep the distribution country-agnostic, and say so in `meta`.
- [x] 4.6 Handler tests for every scenario in `specs/market-insights/spec.md`, including
      the empty-`skills` case for a role below the floor (200, never 404).

## 5. Signed-in coverage

- [x] 5.1 When the request carries a session, read `userprofile.skills` and compute
      coverage with `jobmatch.Compute` over the role's ranked skills. Do not write a
      second matcher.
- [x] 5.2 Report per skill: held / adjacent (naming the skill matched through, via
      `skilladjacency`) / missing, plus a held-out-of-total count.
- [x] 5.3 A signed-in caller with no profile skills gets a coverage section reporting zero
      held — never an absent section, which a client cannot distinguish from signed-out.
- [x] 5.4 Set `Cache-Control: private` whenever a coverage section is present, and assert
      it in a test. The sibling insights routes set a shared-cache header by default.
- [x] 5.5 Handler tests for every scenario in `specs/role-skill-coverage/spec.md`,
      including the anonymous path returning 200 with no coverage.

## 6. Web

- [x] 6.1 Add the client method in `web/src/lib/api.ts` for the single-role read.
- [x] 6.2 Add `web/src/routes/insights/roles/[category]/[seniority]/+page.server.ts`,
      gated by the same `loadInsightsGate` / `isCovered` read the sibling pages use, and
      404 for a pair that does not clear the gate.
- [x] 6.3 Build `+page.svelte`: role header with open count and growth, the ranked skill
      list, and a link into the filtered jobs search for that role. Follow the existing
      insights pages' layout rather than inventing one.
- [x] 6.4 Label the column "mentioned in", never "required by". This wording is normative
      in the spec.
- [x] 6.5 State the `sample_size` the distribution was measured over, and never word the
      page as describing the role's market — only 39.0% of open tech postings state a
      seniority (task 1.1). Normative in the spec.
- [x] 6.6 Render the CATEGORY-only distribution beside the role's, read from the existing
      `/api/v1/insights/skills?category=…` (98.7% coverage). It costs no new rollup and it
      is what stops the 39%-coverage slice from standing alone.
- [x] 6.7 Overlay the coverage for a signed-in visitor: held / adjacent (showing the
      neighbour) / missing, plus the held-out-of-total line. Anonymous visitors see the
      aggregate alone with no empty column.
- [x] 6.8 Link each seniority row on `/insights/roles/[category]` to its new leaf page.
- [x] 6.9 Extend `insightsPaths` and `sitemap-insights.xml` to list the (category,
      seniority) leaves that clear the gate — and only those, matching what the route
      serves.
- [x] 6.10 `pnpm check:dead` passes (knip gates unused exports, types included). It caught
      three: `isSeniority` and two wire types nothing imports BY NAME, reached only through
      `InsightRole`. All three are now file-private. Note the local caveat — knip also
      reports `extension/` here because that package is npm-managed and not installed in a
      fresh worktree; CI installs it, which is why this gate is CI-only.

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
