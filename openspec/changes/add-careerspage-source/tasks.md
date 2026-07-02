## 1. Fixtures

- [x] 1.1 Fixtures for the adapter test. (Followed the codebase convention: inline
  HTML constants in `careerspage_test.go` — `careerspageListingHTML`,
  `careerspageDetailHTML`, `emptyListingHTML` — plus the shared `routedHTTP` fake,
  mirroring `icims_test.go`/`freshteam_test.go`; no separate `testdata/` files.)

## 2. Adapter (TDD, one behavior at a time)

- [x] 2.1 `careerspageJobID` extracts the job UUID from a `/jobs/<uuid>` URL and
  returns "" for sub-action/non-job URLs (compiled regexp).
- [x] 2.2 Listing parse — the adapter collects every canonical `/jobs/<uuid>` detail
  URL via `jobLinks`, excluding `/refer` and `/apply` sub-actions.
- [x] 2.3 Detail parse — `detail` maps the `JobPosting` `ld+json` to a `Job` (title,
  sanitized HTML description, location from `jobLocation.address`, company via
  `firstNonEmpty(hiringOrganization.name, e.Company)`, `PostedAt` via `parseRFC3339`,
  `ExternalID` = UUID with an empty-id drop-guard, `URL` = detail URL); a page with
  no `JobPosting` yields ok=false.
- [x] 2.4 Pagination — `Fetch` pages `?page=N` until a page yields no new job links,
  then fans out details via `fetchDetails`; `careerspageMaxPages` guards a
  never-ending listing.
- [x] 2.5 `Provider()` returns `"careerspage"`; the adapter is board-based and appears
  in `All()` / `FilterableProviders()` (registration test).

## 3. Registry + board file

- [x] 3.1 Added `NewCareerPage(c)` to `sources.All` in `internal/sources/source.go`.
- [x] 3.2 Created `sources/careerspage.yml` with the validated David Joseph & Company
  entry (`davidjoseph-co`); passes config validation.

## 4. Verify

- [x] 4.1 `go build ./...` / `go vet ./internal/sources/` / `gofmt -l` clean;
  `go test ./internal/sources/` green.
- [x] 4.2 Live smoke against `davidjoseph-co`: returned 9 real open postings with
  populated title/description/location/URL/posted-at in ~10s; throwaway harness
  discarded.
