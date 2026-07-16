# Tasks

Each task runs the spec-driven-tdd micro-cycle (RED → GREEN → REFACTOR → simplify →
re-test → review) before it is checked off. Backend uses Go tests; pure web logic uses
vitest; Svelte pages are verified via svelte-check + a visual screenshot pass.

## 1. API: category-scoped roles

- [x] 1.1 Add an optional `category` param to the `ListInsightsRoles` read query in
      `internal/db/queries/insights.sql` (filter `WHERE category = @category` when
      non-empty; unchanged when empty). Regenerate sqlc; `go build ./...`.
- [x] 1.2 Wire the `category` param through `InsightsRoles` handler with vocab
      validation (reuse `parseCategory`); echo it in `meta`. Unit-test the parse/echo
      and add the `category` filter assertion to the roles integration test.

## 2. API: per-category salary (all seniorities)

- [x] 2.1 Add a read that returns every seniority's salary bands for a category in one
      call (query over `insights_salary_stats` for `category=@category`, all
      seniorities, country `''`), preserving sample suppression. Regenerate sqlc.
- [x] 2.2 Expose it via the salary handler (a `category`-only mode returning
      per-seniority bands) or a sibling route per design D2/Open-Questions; validate
      params; extend the salary integration test to assert per-seniority grouping.

## 3. Web: covered-set quality gate

- [x] 3.1 Add a helper (e.g. `web/src/lib/insights.ts`) that fetches the insights data
      and derives the covered-category set + per-category view models, applying the
      gate thresholds (min open jobs / salary sample floor). Pure logic — vitest-cover
      the gate (covered vs excluded, empty-data case).

## 4. Web: shared page component + hub

- [x] 4.1 Build the shared insights page layout component (breadcrumb, data-driven
      auto-intro, data section slot, internal-link rail) as pure/presentational as
      possible; svelte-check clean.
- [x] 4.2 Add `/insights` hub route (`+page.server.ts` + `+page.svelte`): lists covered
      categories × insight types from the gate helper; server-rendered links.

## 5. Web: per-category routes

- [x] 5.1 `/insights/salary/[category]` — `+page.server.ts` loads the category salary
      bands (404 if not covered), `+page.svelte` renders bands by seniority + intro +
      links + `Cache-Control: s-maxage=3600`.
- [x] 5.2 `/insights/skills/[category]` — same shape over the skills read.
- [x] 5.3 `/insights/roles/[category]` — same shape over the category-scoped roles read.

## 6. Web: SEO metadata & structured data

- [x] 6.1 Per page: unique `<title>`/meta description, canonical, Open Graph, and
      JSON-LD (BreadcrumbList + Dataset) rendered server-side; internal links to the
      `/jobs` filtered view, sibling categories, and the other insight types.
- [x] 6.2 Verify the rendered HTML carries the structured data and links (svelte-check
      + a headless-Chrome screenshot/DOM check on a covered category and a 404 on a
      gated-out one).

## 7. Sitemap

- [x] 7.1 Add `web/src/routes/sitemap-insights.xml` listing only covered pages (hub +
      covered categories × insight types), and reference it from the sitemap index
      (`sitemap.xml`). Assert (test/manual) an uncovered category is absent.

## 8. Docs & verify

- [x] 8.1 Document the two API additions in `docs/API.md` and `web/static/openapi.yaml`.
- [x] 8.2 `go build ./... && go vet ./... && go test ./...`; run insights integration
      tests; `npm run check` in web.
- [x] 8.3 verification-before-completion: run the app against a seeded DB with rollups
      populated, load `/insights`, a covered category's three pages, `sitemap-insights.xml`,
      and a gated-out category (expect 404); confirm server-rendered content + JSON-LD.
