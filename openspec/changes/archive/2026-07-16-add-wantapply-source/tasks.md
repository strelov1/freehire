## 1. Sitemap discovery & vacancy filter

- [x] 1.1 Add `internal/sources/testdata/wantapply-sitemap.xml` and `internal/sources/testdata/wantapply-job.html` fixtures (a small sitemap with vacancy + `/company/*` + `/jobs/*` + static locs, and one real vacancy detail page with its `JobPosting` JSON-LD).
- [x] 1.2 Implement sitemap fetch + parse and the vacancy-slug filter (single-segment path, excluding the static set and `/company/*`, `/jobs/*`); yields candidate `{slug, url}`. Test: fixture sitemap → only the vacancy slug survives; reserved paths are dropped.

## 2. JobPosting → Job mapping

- [x] 2.1 Implement detail-page mapping via `jsonld.LDJobPosting`: title, company (`hiringOrganization.name`), sanitized description, `datePosted`→PostedAt, `jobLocation[].address`→Location, `jobLocationType==TELECOMMUTE`→Remote+WorkMode `remote`, `employmentType[0]`→enrich employment-type vocab. Test against `wantapply-job.html`: fields mapped; remote case sets WorkMode; a page with no `JobPosting`/no company is dropped.

## 3. Adapter: Fetch + HydratingSource FetchNew

- [x] 3.1 Implement `wantapply` adapter type with `Provider()`, `boardless()`, `aggregator()` markers and the plain `Fetch` list-only fallback (sitemap candidates without detail). Test: `Provider()=="wantapply"`; `Fetch` yields one candidate per vacancy loc.
- [x] 3.2 Implement `FetchNew(ctx, e, seen)`: for unseen slugs fetch+map detail (bounded concurrency, single-failure isolation); for seen slugs emit `Job{SeenRefresh:true}` carrying Title/Company/URL/ExternalID with no detail request. Tests: unseen→hydrated Job; seen→SeenRefresh with no fetch; one detail failure is isolated and the crawl continues.

## 4. Registration & board wiring

- [x] 4.1 Register `wantapply` in `sources.All`. Test: `TestWantapplyRegisteredInAll` asserts registration, boardless, and aggregator markers (and that it is NOT self-closing).
- [x] 4.2 Add `sources/wantapply.yml` with a single boardless entry (`company: Wantapply`, `provider: wantapply`); confirm it validates against the registry (config validation / `go run ./cmd/ingest` dry check).

## 5. Telegram channels

- [x] 5.1 Add four `kind: board` channels to `sources/telegram.yml` (`wantapply_managers`, `wantapply_design`, `wantapply_analytics`, `wantapply_qa_jobs`), keeping `wantapply_marketing`. Verify the file parses.

## 6. Verify

- [x] 6.1 `go build ./... && go vet ./... && go test ./internal/sources/` green; run the adapter against live `wantapply.cy` once (small bounded sample) to confirm real sitemap+detail parse end-to-end.
