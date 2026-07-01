## 1. Backend: keyset sitemap queries (sqlc)

- [x] 1.1 Write a failing DB integration test (`//go:build integration`) seeding open/closed jobs and companies, asserting: job keyset slices page by `id > cursor` and skip closed jobs; company keyset slices page by `slug > cursor`; the boundary-cursor queries return the id/slug at each Nth row (RED)
- [x] 1.2 Add sqlc queries `ListJobSitemap`, `JobSitemapBoundaries`, `ListCompanySitemap`, `CompanySitemapBoundaries` in `internal/db/queries/{jobs,companies}.sql`, run `make sqlc`, commit generated code → test GREEN

## 2. Backend: sitemap handlers + routes

- [x] 2.1 Write a failing handler integration test: the job/company sitemap slice endpoints return slim `{slug, updated_at}` (+ next cursor) JSON, honor the keyset cursor + limit, and resolve without being captured by the `/:slug` route; the boundary endpoints return the chunk cursors (RED)
- [x] 2.2 Implement handlers in `internal/handler/sitemap.go` and register the routes in `handler.go` before the `/jobs/:slug` and `/companies/:slug` catch-alls → GREEN

## 3. Frontend: sitemap index + sub-sitemaps

- [x] 3.1 Add slim API-client methods (job/company sitemap slice + boundaries) in `web/src/lib/api.ts`
- [x] 3.2 Extract the shared XML escaping / `<url>` builders and rewrite `web/src/routes/sitemap.xml/+server.ts` to emit a `<sitemapindex>` built from the boundary cursors + a static-pages entry
- [x] 3.3 Add sub-sitemap routes: `sitemap-pages.xml`, `sitemap-jobs/[cursor].xml`, `sitemap-companies/[cursor].xml` (each a `<urlset>` chunk)
- [x] 3.4 Verify frontend: `svelte-check` clean + `npm run build`; manual `curl` of the index and one job/company chunk against a running stack (no test runner in `web/` — verification is build + curl)

## 4. Cleanup + full verification

- [x] 4.1 Remove the old 5,000-row caps and truncation warnings; confirm no orphaned helpers remain
- [x] 4.2 Full pass: `go build ./... && go vet ./...`, unit + integration tests, `svelte-check`, and an end-to-end fetch of `/sitemap.xml` → a sub-sitemap chunk
