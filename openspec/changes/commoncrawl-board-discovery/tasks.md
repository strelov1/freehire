## 1. Shared Common Crawl CDX helper

- [ ] 1.1 Add `cmd/harvest-boards/commoncrawl.go`: fetch `index.commoncrawl.org/collinfo.json`,
      take the 3 most recent snapshots.
- [ ] 1.2 Implement `commonCrawlCandidates(ctx, c httpClient, hostPrefix string) ([]string, error)`:
      for each snapshot, page `<cdx-api>?url=<hostPrefix>/*&output=json&page=N` (bounded page
      count, mirroring `gupyMaxOffset`'s safety-cap pattern), parse JSON-lines records, extract
      each URL's first non-empty path segment lowercased, collect into a deduplicated set.
- [ ] 1.3 One failing snapshot logs and is skipped; return an error only if every snapshot fails.
- [ ] 1.4 Unit tests: slug extraction (bare path, path with query string/UTM params, trailing
      slash, uppercase, no path) and CDX JSON-lines response parsing, using `httptest` fixtures
      per the existing `*_test.go` pattern in this package.

## 2. Wire discovery into the Greenhouse and Ashby probers

- [ ] 2.1 Add `func (greenhouseProber) discover(ctx context.Context, c httpClient) ([]string, error)`
      calling `commonCrawlCandidates` with `boards.greenhouse.io`; note in a comment that
      validation happens against the separate stable `boards-api.greenhouse.io`, so the
      frontend's redirect to `job-boards.greenhouse.io` (visible in raw CDX records) doesn't
      affect candidate validity.
- [ ] 2.2 Add `func (ashbyProber) discover(ctx context.Context, c httpClient) ([]string, error)`
      calling `commonCrawlCandidates` with `jobs.ashbyhq.com`.
- [ ] 2.3 Confirm no change is needed to `leverProber` (out of scope — Lever disallows CCBot).

## 3. Verify end-to-end

- [ ] 3.1 `go build ./...` and `go vet ./...`.
- [ ] 3.2 `go vet -tags=integration ./...` (repo-wide pre-push guard).
- [ ] 3.3 `go test ./cmd/harvest-boards/...`.
- [ ] 3.4 Manual dry run: `go run ./cmd/harvest-boards greenhouse` and `... ashby` with no seed
      file against the live Common Crawl API, confirm candidates are discovered, live-validated,
      and would append new boards not already in `sources/greenhouse.yml` / `sources/ashby.yml`.
