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

What IS cheap is a list scoped to ONE company, given an index built for it. The
existing `jobs_company_slug_idx` (`company_slug, posted_at DESC NULLS LAST, id DESC`,
migration 0001) is not partial, so it does reach closed rows — but `closed_at`,
`duplicate_of` and `is_private` are heap filters on it, and page 1 walks every OPEN
posting of the company before finding a closed one. Measured on prod, cold: dominos
skipped 14.5k open rows in 5.4s, autozone 16.4k in 14.8s, and lowes holds 23.6k; a
~10k API offset took 6.4s. So this design adds one partial index (migration 0194, below).

Revisit closed postings in search once the host has headroom.

## Design

### Query — `ListClosedJobsByCompany` (`internal/platform/db/queries/jobs.sql`)

```sql
SELECT *
FROM jobs
WHERE company_slug = $1 AND closed_at IS NOT NULL AND duplicate_of IS NULL AND NOT is_private
ORDER BY closed_at DESC, id DESC
LIMIT $2 OFFSET $3;
```

Served by a new partial index, built `CONCURRENTLY` (migration 0194, `migrate: no-transaction`):

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_closed_company_closed_at_idx
    ON jobs (company_slug, closed_at DESC, id DESC)
    WHERE closed_at IS NOT NULL AND duplicate_of IS NULL AND NOT is_private;
```

- Same exclusions as `ListJobsByCompany`: role-cluster reposts (`duplicate_of`) and
  private pasted JDs (`is_private` — the leak that query's comment records).
- Partial on exactly the query's WHERE, so the index holds only rows the query can
  return and the walk starts at the first one, whatever the company has open.
- The ORDER BY matches the index exactly, so a company with hundreds of thousands of
  closed rows is a bounded index walk, not a sort.
- Most recently closed first: the order a reader of a closed list wants, and — once
  the index exists — no more expensive than ordering by posting date.

### API — `GET /api/v1/companies/:slug/closed-jobs`

- Registered beside `GET /companies/:slug` in `internal/api/handler/companies.go`,
  `readLimit`, anonymous.
- The pagination window first (its refusal costs nothing, the rule the other list
  endpoints follow), but a narrower one: `offset + limit` may not exceed 2000
  (`closedJobsWindow`, the web's `FEED_WINDOW`), not the shared 10000. Even indexed, a
  deep offset is one random heap fetch per skipped closed row. Then `lookupCompany` (unknown slug → 404, retired slug → 301
  to the canonical one).
- Fetches `limit + 1` rows and answers
  `{"data": [job...], "meta": {"limit": N, "offset": M, "has_more": bool}}`.
  No total: an exact count over a large employer's closed rows is the cost this design
  avoids.
- Jobs go through `jobview.FromRows`, so every item carries its `closed_at`.
- Documented in `web/src/lib/docs/api-spec.ts` (and the generated OpenAPI).

### Web — `/companies/<slug>/closed`

- `+page.server.ts` loads the company (for the header) and one page of closed jobs;
  `?page=N` with `<a href>` previous/next links driven by `has_more`, and no "Older"
  link past `MAX_PAGE` (the page after it is a 404).
- Rows render through the existing job row component, which already hides freshness
  badges for a closed posting; each row also states the close date.
- `noindex`, and `/companies/*/closed` is disallowed in `robots.txt`
  (`web/src/lib/robots.ts`): millions of closed-posting list pages are not something to
  offer a crawler, nor to spend its budget on. The job detail pages remain as they are.
- It is the company page's fourth tab ("Closed", beside Jobs / Hiring / Discussion in
  `CompanyTabs.svelte`) — the tabs are already separate routes, so this is the existing
  shape rather than a new one. The tab is shown always; the page states plainly when
  there are none.

### Tests

- Integration test for the query: returns only closed, non-duplicate, non-private rows
  of the given company, most recently closed first (seeded with `posted_at` and
  `closed_at` in opposite orders, so a fallback to posting order fails it).
- Handler integration test: 404 for an unknown slug, `has_more` true/false around the
  page boundary, the pagination-window refusal at its own 2000-row window (added to
  `pagination_window_routes_test.go`, which now carries each route's window).
- Web: the loader only composes existing, tested helpers (`parsePage`, `pageOffset`,
  `pageWithinWindow`), so it is checked by `svelte-check` plus a run in the browser.

## Out of scope

- Filters or text search over closed postings.
- Closed postings in `/jobs` search or Meilisearch.
- A closed-count badge on the company page.
