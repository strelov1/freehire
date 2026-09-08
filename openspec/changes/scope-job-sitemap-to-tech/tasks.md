## 1. The filter

- [x] 1.1 Give `sitemapPage` (`internal/search/search/sitemap.go`) a `filter string` parameter, and
      assign `meilisearch.DocumentsQuery.Filter` only when it is non-empty. The guard is
      load-bearing, not tidiness: `Filter` is an `interface{}` with `json:"filter,omitempty"`, and
      `omitempty` drops an interface only when it is NIL — an interface holding `""` serializes as
      `"filter":""`. Assigning unconditionally would therefore send an empty filter expression on
      the company path, which issues a different request than the one it issues today.
- [x] 1.2 Add the predicate as a named constant beside the two readers, with the reasoning from
      design.md in its comment (why `is_tech`, why `likely-evergreen` and not `stale`, why the
      unknown bucket is excluded):
      `is_tech = "tech" AND reality.class != "likely-evergreen"`.
- [x] 1.3 Pass it from `ListSitemapPage`; pass `""` from `ListCompanySitemapPage`. Leave
      `CountSitemapDocuments` and `CountCompanySitemapDocuments` alone — they delegate, and that
      is what keeps the count and the pages from disagreeing.
- [x] 1.4 Replace the stale performance note in `sitemapPage`'s doc comment with the 2026-09-08
      measurement (2.01M documents: deepest unfiltered page 4.43s, deepest filtered 4.07s,
      count 0.035s, against the route's 10s fetch timeout). Keep the paragraph's point — the
      engine addresses an offset directly instead of walking to it — and correct only the
      figures and the margin they imply.

## 2. Tests

- [x] 2.1 Integration test in `internal/search/search` (build tag `integration`), in the shape of
      `TestIntegration_IsTechFilter`: `startMeili(t)`, `EnsureIndex`, then FIVE fixtures through
      `FromJob` — a `fresh` tech job, a `stale` tech job, a `non_tech` job, one with `is_tech`
      NULL, and a tech job whose `doc.Reality` is `likely-evergreen` (the caller attaches
      reality, so the fixture sets it directly rather than driving `jobreality`).
      The `stale` tech fixture is the one this change most needs pinned: the design turns on
      `stale` being an age bucket rather than a liveness verdict, so a future filter that
      excluded it would be caught here rather than by a two-thirds drop in prod coverage.
- [x] 2.2 Assert `ListSitemapPage` names both tech postings and none of the other three, that
      its `total` is 2, and that `CountSitemapDocuments` reports the same 2 — the agreement
      between the count and the pages is the property the tiling depends on. Cover the
      past-the-end offset too, since that is what a crawler holding the old, wider index asks
      for once the chunk count drops.
- [x] 2.3 Assert `ListCompanySitemapPage` over the companies index is untouched by the change:
      every indexed company still comes back.

## 3. Spec and docs

- [x] 3.1 Land the `web-ssr-seo` delta: the job sub-sitemap's scope is the tech slice, and the
      requirement's superseded sentence about listing "the freshest open jobs up to the per-file
      limit, rather than the full catalogue" goes with it — that follow-up shipped when the
      sitemap moved onto the search index.
- [x] 3.2 Check whether `web/src/lib/sitemap.ts`'s comment ("the index already holds exactly the
      postings worth crawling") still reads correctly. It becomes true rather than aspirational;
      adjust only if the wording now misleads.

## 4. Verification

- [x] 4.1 `gofmt -l .` prints nothing; `go vet ./...`; `go test ./...`;
      `go vet -tags=integration ./...`.
- [x] 4.2 `go test -tags=integration ./internal/search/search/` (needs Docker; testcontainers).
- [ ] 4.3 After deploy, confirm on prod that `/sitemap.xml` lists 68 job sub-sitemaps, that the
      first and last both return a non-empty `<urlset>`, and that a retired offset
      (e.g. `?offset=1000000`) returns an empty `<urlset>` rather than an error.
