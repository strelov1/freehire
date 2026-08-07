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
      First run (before 3.5): greenhouse 29 live boards appended, ashby 602 — but 5 and 398 of
      those respectively turned out to be case-variant duplicates of already-tracked boards
      (see 3.5). A handful of individual CDX pages 504'd or timed out mid-run; per-page error
      tolerance logged and skipped them without aborting either run, as designed.
- [x] 3.5 Bug found while reviewing the run for merge conflicts: both APIs are
      case-insensitive (verified live), so the case-preserving discovery was producing
      same-company duplicates under different casing — 415 collision groups in
      `sources/ashby.yml`, 5 in `sources/greenhouse.yml`, none pre-existing. Fixed by adding
      `dedupKey` (case-fold) to `greenhouseProber`/`ashbyProber`, mirroring `workdayProber`'s;
      `newBoards` already dedupes through it. Reverted the first run's `sources/*.yml` changes
      and re-ran clean: greenhouse 24 live boards appended (29 minus the 5 duplicates), ashby
      178 (602 minus 424 duplicates — most of the drop was against boards already tracked
      under different case, not within this run). Zero case-collisions remain in either file
      after the re-run.
