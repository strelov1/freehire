## 1. Backend — member-growth endpoint (user-growth-stats)

- [x] 1.1 Add a hand-written query to `internal/db/queries/stats.sql` grouping `users.created_at` by UTC day with a running cumulative total (`SUM() OVER (ORDER BY day)`); run `make sqlc` and commit the generated `internal/db` code
- [x] 1.2 Add a `UserGrowth` handler in `internal/handler/stats.go` (a `JobsActivity` sibling) returning `{"data": [{date,total}]}`; empty catalogue → `{"data": []}`; aggregate-only, no PII
- [x] 1.3 Wire `GET /stats/user-growth` in `handler.Register` as an unauthenticated public read next to `/stats/jobs-activity`
- [x] 1.4 Handler tests: cumulative monotonicity, empty-catalogue 200, no-auth access (mirror the `job-activity-stats` test shape)

## 2. Frontend — API client + types

- [x] 2.1 Add the `UserGrowthPoint` type and an `api.userGrowth()` method (+ `serverApi` variant) in `web/src/lib/api.ts` / types, mirroring `jobsActivity`
- [x] 2.2 Unit-test the pure client mapping if any transformation is added (vitest)

## 3. Frontend — /open page

- [x] 3.1 `web/src/routes/open/+page.server.ts`: concurrent best-effort SSR fan-out (`Promise.allSettled`) over totals, jobs-activity, facets, user-growth, and a cached GitHub fetch; each slice resolves to null on failure; set `cache-control` via `setHeaders`
- [x] 3.2 Module-level TTL (~1h) in-memory cache for the GitHub call to stay under the 60/hr unauth limit; on failure return null (section falls back to a plain GitHub link)
- [x] 3.3 `web/src/routes/open/+page.svelte`: render sections A (scale stat-strip) · B (`ActivityBars` from jobs-activity) · F (`ActivityBars` from cumulative user-growth) · C (facet-distribution bars from `/jobs/facets`) · D (GitHub stars/forks/contributors + MIT badge + "add a source = one PR" CTA), reusing the HomeView stat-strip idiom and the site aesthetic (Inter, monochrome, `// section` mono labels)
- [x] 3.4 Each headline figure links to the public API endpoint that produced it
- [x] 3.5 Per-section fallback UI when its slice is null (best-effort degradation), including the GitHub-unavailable case
- [x] 3.6 `Seo` title/description for `/open`; intro copy ("all our numbers, live") noting figures come live from the public API

## 4. Navigation + verification

- [x] 4.1 Add an `/open` link to the footer (and/or nav/menu), matching the existing `/about`·`/trends`·`/blog` chrome
- [x] 4.2 Verify: `go build ./... && go vet ./... && go test ./...` (backend) and `npm run check` (web) pass; manually load `/open` and confirm every section renders, figures link to their API, and killing the GitHub call still renders the page
