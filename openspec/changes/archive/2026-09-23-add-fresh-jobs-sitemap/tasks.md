## 1. Search reader

- [x] 1.1 Add `Client.ListFreshSitemapPage(ctx, offset, limit)` in
  `internal/search/search/sitemap.go`: one page of `SitemapDocument` drawn through
  Meilisearch `/search` with `Sort: ["created_at:desc"]`, `Filter: jobSitemapFilter`,
  and only the slug and lastmod attributes retrieved. Test first: the request it
  builds carries the descending `created_at` sort and exactly the same filter string
  the paged reader uses — a test comparing against the literal would pass if the two
  drifted apart, so assert against `jobSitemapFilter` itself.

## 2. API route

- [x] 2.1 Add `freshSitemapMaxOffset` (30,000) and `GET /jobs/sitemap/fresh` in
  `internal/api/handler/sitemap.go`, behind the same `publicReadLimiter` as the other
  sitemap routes, serving `jobSitemapChunk` entries. Test first: an offset above the
  bound yields an empty `data` array and HTTP 200, never an error, and an offset
  within the bound is passed through unchanged.

## 3. SPA route and index

- [x] 3.1 Add `web/src/routes/sitemap-jobs-fresh.xml/+server.ts`, mirroring
  `sitemap-jobs.xml`: read `?offset=`, call the new API method, render `urlsetXml`.
  NOT covered by a test of its own, deliberately: no SvelteKit sitemap route in this
  repo has one, the body is a two-line `.map` over what the API returned, and order
  preservation is asserted one layer down where it can actually be broken
  (`TestFreshJobSitemapPreservesOrder`). Adding the repo's first route test here would
  be infrastructure for the cheapest line in the change.
- [x] 3.2 Add `FRESH_SITEMAP_CHUNKS = 4` to `web/src/lib/sitemap.ts` and emit the
  four fresh sub-sitemap entries **before** the paged job entries in the sitemap
  index. Test first: the index lists four fresh `<loc>`s and every one of them
  appears at a lower position than the first paged job `<loc>`.

## 4. Wire the client method

- [x] 4.1 Add the `sitemapJobsFresh(offset, limit)` method to the SPA's server API
  client alongside `sitemapJobs`, and regenerate contracts if the repo's generated
  client covers this route. Verify `pnpm check:dead` has nothing to say about the new
  exports.

## 5. Verify and ship

- [x] 5.1 Run the full local gate: `gofmt -l .` silent, `go vet ./...`,
  `go test ./...`, `go vet -tags=integration ./...`, and the web unit tests.
- [x] 5.2 Open the PR, get CI green, merge.
- [x] 5.3 Deploy to prod (`release.sh`), then verify on the live host:
  `/sitemap.xml` lists the four fresh entries ahead of the paged ones; the fresh
  sub-sitemap returns valid XML; the deepest fresh chunk (`?offset=30000`) answers
  inside the SSR timeout. **Check the ORDER against `jobs.created_at` in Postgres for
  the slugs the file lists, NOT against the `<lastmod>` values in it** — `<lastmod>` is
  `updated_at`, the file is ordered by `created_at`, and the two disagree routinely, so
  a descending-`<lastmod>` check would pass against a wrongly-ordered file and fail
  against a correct one.
- [x] 5.4 Record the crawl-coverage baseline (17.9%, 2026-09-09..19) and the exact
  commands to re-measure it in 7 days, so the follow-up is not re-derived.
