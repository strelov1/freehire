## 1. eRecruiter adapter

- [x] 1.1 Add a fixture-backed test (`internal/sources/erecruiter_test.go`) for the list
  parse: JSONP `({"htm":"<tr ...>"})` → rows with `offerId`/`externalJobOfferId`/
  `externalJobOfferRegionId`/`comId`/title/city, plus the `tr` total from the marker row.
- [x] 1.2 Add a fixture-backed test for the detail parse: `Offer.aspx` HTML →
  title/company/location/description; missing/closed detail is skipped.
- [x] 1.3 Implement `internal/sources/erecruiter.go`: `Provider() == "erecruiter"`,
  board-based (not boardless), `Fetch` reads `GetHtml.ashx?cfg=<board>&grid=rows&pn=<n>`
  via `GetText`, unwraps the JSONP, parses rows, and fetches each offer's `Offer.aspx`
  detail; maps to `Job` with `ExternalID = externalJobOfferId` and the `Offer.aspx` URL.
- [x] 1.4 Implement paginated collection: read `tr` total from page 1, page until the
  total is collected or a page yields no offer rows (or a non-advancing page), under a
  bounded page cap. Detail fetches run through the shared `fetchDetails` worker pool.
- [x] 1.5 Parse defensively — a posting whose title/description can't be extracted is
  skipped without aborting the board; sanitize the HTML description (existing helper).
- [x] 1.6 `go build ./... && go vet ./... && go test ./internal/sources/` green.

## 2. Registry and board file

- [x] 2.1 Register `NewErecruiter(c)` in `sources.All` (`internal/sources/source.go`).
- [x] 2.2 Add `sources/erecruiter.yml` with the board-file header and a live-validated seed
  entry (Atalian Poland cfg).
- [x] 2.3 Confirm the adapter crawls a seed board end-to-end (32 live postings fetched with
  real title/location/description).

## 3. cfg harvester

- [x] 3.1 Add a test for the cfg extractor: careers-page HTML → `cfg` token, and a page
  without the `Code.ashx?cfg=` widget yields no token (skipped).
- [x] 3.2 Implement `cmd/harvest-erecruiter/main.go`: read company careers URLs (or
  domains) from input, fetch each, extract `cfg`, live-validate via the adapter, and print
  `sources/erecruiter.yml` entries for the valid ones; skip the rest without aborting.
- [ ] 3.3 Run the harvester over the justjoin-mined eRecruiter company set (needs each
  company's careers URL) and fold the validated entries into `sources/erecruiter.yml`.
  Deferred — data-gathering follow-up; the tool and adapter are done and verified live.

## 4. Ops wiring

- [x] 4.1 `cmd/harvest-erecruiter` is a run-once host tool (like `cmd/harvest-boards`), not
  a prod binary — left out of the Dockerfile and documented in the board-file header.
- [ ] 4.2 Wire a per-provider ingest cron for `sources/erecruiter.yml`. Deferred — cron
  schedules live in the `freehire-ops` repo, not this codebase.

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green (46 packages).
- [x] 5.2 Manually crawled a seed board and confirmed postings carry real
  title/location/description and dedup on the stable `externalJobOfferId`.
- [x] 5.3 `openspec validate add-erecruiter-source` passes.
