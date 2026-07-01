## 1. eRecruiter adapter

- [ ] 1.1 Add a fixture-backed test (`internal/sources/erecruiter_test.go`) for the list
  parse: JSONP `({"htm":"<tr ...>"})` → rows with `offerId`/`externalJobOfferId`/
  `externalJobOfferRegionId`/`comId`/title/city, plus the `tr` total from the marker row.
- [ ] 1.2 Add a fixture-backed test for the detail parse: `Offer.aspx` HTML →
  title/company/location/description + apply `WebID` URL; missing/closed detail is skipped.
- [ ] 1.3 Implement `internal/sources/erecruiter.go`: `Provider() == "erecruiter"`,
  board-based (not boardless), `Fetch` reads `GetHtml.ashx?cfg=<board>&grid=rows&pn=<n>`
  via `GetText`, unwraps the JSONP, parses rows, and fetches each offer's `Offer.aspx`
  detail; maps to `Job` with `ExternalID = externalJobOfferId` and the `Offer.aspx` URL.
- [ ] 1.4 Implement paginated collection: read `tr` total from page 1, page until the
  total is collected or a page yields no offer rows, under a bounded page cap.
- [ ] 1.5 Parse defensively — a posting whose title/description can't be extracted is
  skipped without aborting the board; sanitize the HTML description (existing helper).
- [ ] 1.6 `go build ./... && go vet ./... && go test ./internal/sources/` green.

## 2. Registry and board file

- [ ] 2.1 Register `NewErecruiter(c)` in `sources.All` (`internal/sources/source.go`).
- [ ] 2.2 Add `sources/erecruiter.yml` with the board-file header and a small seed set of
  hand-validated `company` + `board`=cfg entries.
- [ ] 2.3 Confirm `cmd/ingest sources/erecruiter.yml` validates the board file against the
  registry and crawls a seed board end-to-end (postings upserted).

## 3. cfg harvester

- [ ] 3.1 Add a test for the cfg extractor: careers-page HTML → `cfg` token, and a page
  without the `Code.ashx?cfg=` widget yields no token (skipped).
- [ ] 3.2 Implement `cmd/harvest-erecruiter/main.go`: read company careers URLs (or
  domains) from input, fetch each, extract `cfg`, live-validate against
  `GetHtml.ashx?cfg=<cfg>&grid=rows&pn=1`, and print `sources/erecruiter.yml` entries for
  the valid ones; skip the rest without aborting.
- [ ] 3.3 Run the harvester over the justjoin-mined eRecruiter company set and fold the
  validated entries into `sources/erecruiter.yml`.

## 4. Ops wiring

- [ ] 4.1 Add `cmd/harvest-erecruiter` to the Dockerfile build if it needs to run in prod;
  otherwise document it as a local harvest tool.
- [ ] 4.2 Wire a per-provider ingest cron for `sources/erecruiter.yml` (one schedule, as
  for other board files).

## 5. Verification

- [ ] 5.1 `go build ./... && go vet ./... && go test ./...` green.
- [ ] 5.2 Manually crawl 2–3 seed boards and confirm postings carry real
  title/location/description and dedup on recrawl (stable `externalJobOfferId`).
- [ ] 5.3 `openspec validate add-erecruiter-source` passes.
