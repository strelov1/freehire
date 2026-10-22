# A company's closed postings

## Goal

Let a visitor see the postings a company has CLOSED — "what has this employer been
hiring for" — without putting closed rows into Meilisearch.

## Why not search

The search index holds open postings only, and widening it is not affordable today.
Measured on prod 2026-10-22:

- `jobs` index: 3.79M documents, Meilisearch database 79 GB.
- Canonical rows closed in the last 30 days: 1.64M; last 90 days: 4.63M.
- The host runs at load ~23 on 12 cores with swap full.

Thirty days of closed postings would add roughly 40-50% to the index and to every
rebuild, on a host that is already saturated. A Postgres-wide filter over closed rows
is no alternative either: a single `count(*)` over `jobs` with a `closed_at` predicate
took longer than five minutes.

What IS cheap is a list scoped to ONE company: `jobs_company_slug_idx`
(`company_slug, posted_at DESC NULLS LAST, id DESC`, migration 0001) is not partial, so
it already covers closed rows. No migration is needed.

Revisit closed postings in search once the host has headroom.

## Design

### Query — `ListClosedJobsByCompany` (`internal/platform/db/queries/jobs.sql`)

```sql
SELECT *
FROM jobs
WHERE company_slug = $1 AND closed_at IS NOT NULL AND duplicate_of IS NULL AND NOT is_private
ORDER BY posted_at DESC NULLS LAST, id DESC
LIMIT $2 OFFSET $3;
```

- Same exclusions as `ListJobsByCompany`: role-cluster reposts (`duplicate_of`) and
  private pasted JDs (`is_private` — the leak that query's comment records).
- The ORDER BY matches the index exactly, so a company with hundreds of thousands of
  closed rows is a bounded index walk, not a sort.
- Ordered by when the posting appeared, not when it closed: `closed_at` has no index
  per company, and the posting date is what the card already shows.

### API — `GET /api/v1/companies/:slug/closed-jobs`

- Registered beside `GET /companies/:slug` in `internal/api/handler/companies.go`,
  `readLimit`, anonymous.
- `pageParams` first (the pagination window refusal costs nothing, the rule the other
  list endpoints follow), then `lookupCompany` (unknown slug → 404, retired slug → 301
  to the canonical one).
- Fetches `limit + 1` rows and answers
  `{"data": [job...], "meta": {"limit": N, "offset": M, "has_more": bool}}`.
  No total: an exact count over a large employer's closed rows is the cost this design
  avoids.
- Jobs go through `jobview.FromRows`, so every item carries its `closed_at`.
- Documented in `web/src/lib/docs/api-spec.ts` (and the generated OpenAPI).

### Web — `/companies/<slug>/closed`

- `+page.server.ts` loads the company (for the header) and one page of closed jobs;
  `?page=N` with `<a href>` previous/next links driven by `has_more`.
- Rows render through the existing job row component, which already hides freshness
  badges for a closed posting; each row also states the close date.
- `noindex`: millions of closed-posting list pages are not something to offer a
  crawler. The job detail pages remain as they are.
- It is the company page's fourth tab ("Closed", beside Jobs / Hiring / Discussion in
  `CompanyTabs.svelte`) — the tabs are already separate routes, so this is the existing
  shape rather than a new one. The tab is shown always; the page states plainly when
  there are none.

### Tests

- Integration test for the query: returns only closed, non-duplicate, non-private rows
  of the given company, newest `posted_at` first.
- Handler integration test: 404 for an unknown slug, `has_more` true/false around the
  page boundary, the pagination-window refusal (added to
  `pagination_window_routes_test.go`).
- Web: the loader only composes existing, tested helpers (`parsePage`, `pageOffset`,
  `pageWithinWindow`), so it is checked by `svelte-check` plus a run in the browser.

## Out of scope

- Filters or text search over closed postings.
- Closed postings in `/jobs` search or Meilisearch.
- A closed-count badge on the company page.
