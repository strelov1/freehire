## 1. Posting mapping (single job from its ld+json)

- [x] 1.1 Add `internal/sources/jobylon_test.go` with a saved real job-page fixture under
  `testdata/` and a RED test asserting the adapter maps one job URL to a `Job`: `<id>` from the
  URL → `ExternalID`, the URL → `URL`, ld+json `title` (HTML-unescaped) → `Title`,
  `hiringOrganization.name` → `Company`, `jobLocation` → `Location`, sanitized `description` →
  `Description`, `datePosted` → `PostedAt`.
- [x] 1.2 Create `internal/sources/jobylon.go`: a `jobylon` struct over a `jobylonHTTP`
  (`XMLGetter` + `HTMLGetter`) client, a `jobylonPosting` ld+json struct (`title`, `description`,
  `datePosted`, `hiringOrganization.name`, `jobLocation` via `schemaPlaces`), a `jobylonJobID`
  URL→id extractor, and a `detail` mapper reusing `ldJobPosting`, `sanitizeHTML`,
  `html.UnescapeString`, `distinctJoin`, `schemaAddress.Location`, `isRemote`, and `parseRFC3339`.
  Make 1.1 green.

## 2. Sitemap enumeration

- [x] 2.1 RED test: a fake `jobylonHTTP` serving a sitemap index (child `sitemap-jobs.xml`) and a
  flat jobs `<urlset>` yields the job locs; a `<loc>` that is not a `/jobs/<id>` page is skipped.
- [x] 2.2 Implement `Fetch` (list-only fallback): resolve the `sitemap-jobs` sub-sitemap of
  `https://emp.jobylon.com/sitemap.xml` via `resolveSubSitemap`, list job locs via
  `sitemapJobLocs(jobylonJobID)`, and detail-fetch each under `fetchDetails`. Make 2.1 green.

## 3. Drop rules

- [x] 3.1 RED test: a job page with no `JobPosting` ld+json is dropped; a posting whose `title` or
  `hiringOrganization.name` resolves empty is dropped; a URL with no numeric id is dropped; the
  remaining postings still map. Include an `employmentType: ["CONTRACTOR"]` fixture and assert the
  posting still maps (the array form must not fail the unmarshal).
- [x] 3.2 Implement the `detail` drop rules (return ok=false on missing ld+json, empty title, or
  empty company; skip a URL with no id). Confirm `jobylonPosting` does not model `employmentType`.
  Make 3.1 green.

## 4. HydratingSource incremental

- [x] 4.1 RED test: `FetchNew` with a `seen` predicate hydrates only unseen ids (detail fetched)
  and emits a seen id as `Job{ExternalID, URL, SeenRefresh: true}` with no detail request; assert
  the seen job carries no description and `SeenRefresh` is set.
- [x] 4.2 Implement `FetchNew` sharing the sitemap enumeration with `Fetch`, branching on
  `seen(id)`. Make 4.1 green.

## 5. Classification & registration

- [x] 5.1 Add the `boardless()` and `aggregator()` markers and `Provider()` returning `"jobylon"`;
  register `jobylon` in `sources.All` (one line, under the multi-company aggregators). Test the
  provider resolves from `All(nil)` and is listed by `FilterableProviders()`. Verify
  `go build ./... && go vet ./...`.

## 6. Board file & docs

- [x] 6.1 Add `sources/jobylon.yml` with a single boardless entry (`company: Jobylon`) and confirm
  `go run ./cmd/ingest sources/jobylon.yml` validates against the registry (fail-fast passes;
  no DATABASE_URL needed for the validation step).
- [x] 6.2 Note the new adapter in `internal/sources/AGENTS.md` if it enumerates adapters by class,
  keeping the surrounding style.

## 7. Verify end-to-end

- [x] 7.1 Run `go test ./internal/sources/...` (all green), then a throwaway live smoke crawl
  against the real sitemap+job pages to confirm postings map and unusable ones are dropped without
  error (not committed).
- [x] 7.2 Discovery deliverable: crawl the live sitemap and produce the distinct list of Jobylon
  companies (name + job count) from each posting's `hiringOrganization.name`, as a throwaway
  report for the user (not committed).
