## 1. Shared Common Crawl CDX helper

- [x] 1.1 Add `cmd/harvest-boards/commoncrawl.go`: fetch `index.commoncrawl.org/collinfo.json`,
      take the 3 most recent snapshots.
- [x] 1.2 Implement `commonCrawlCandidates(ctx, c httpClient, hostPrefix string) ([]string, error)`:
      for each snapshot, page `<cdx-api>?url=<hostPrefix>/*&output=json&page=N` (bounded page
      count, mirroring `gupyMaxOffset`'s safety-cap pattern), parse JSON-lines records, extract
      each URL's first non-empty path segment lowercased, collect into a deduplicated set.
- [x] 1.3 One failing snapshot logs and is skipped; return an error only if every snapshot fails.
- [x] 1.4 Unit tests: slug extraction (bare path, path with query string/UTM params, trailing
      slash, uppercase, no path) and CDX JSON-lines response parsing, using `httptest` fixtures
      per the existing `*_test.go` pattern in this package.

## 2. Wire discovery into the Greenhouse and Ashby probers

- [x] 2.1 Add `func (greenhouseProber) discover(ctx context.Context, c httpClient) ([]string, error)`
      calling `commonCrawlCandidates` with `boards.greenhouse.io`; note in a comment that
      validation happens against the separate stable `boards-api.greenhouse.io`, so the
      frontend's redirect to `job-boards.greenhouse.io` (visible in raw CDX records) doesn't
      affect candidate validity.
- [x] 2.2 Add `func (ashbyProber) discover(ctx context.Context, c httpClient) ([]string, error)`
      calling `commonCrawlCandidates` with `jobs.ashbyhq.com`.
- [x] 2.3 Confirm no change is needed to `leverProber` (out of scope — Lever disallows CCBot).

## 3. Verify end-to-end

- [x] 3.1 `go build ./...` and `go vet ./...`.
- [x] 3.2 `go vet -tags=integration ./...` (repo-wide pre-push guard).
- [x] 3.3 `go test ./cmd/harvest-boards/...`.
- [x] 3.4 Manual dry run: `go run ./cmd/harvest-boards greenhouse` and `... ashby` with no seed
      file against the live Common Crawl API, confirm candidates are discovered, live-validated,
      and would append new boards not already in `sources/greenhouse.yml` / `sources/ashby.yml`.
      Result: greenhouse 2725 candidates / 814 new / 29 live boards appended; ashby 3929
      candidates / 1232 new / 602 live boards appended (0 probe failures/mismatches on both).
      A handful of individual CDX pages 504'd or timed out mid-run; per-page error tolerance
      logged and skipped them without aborting either run, as designed.
