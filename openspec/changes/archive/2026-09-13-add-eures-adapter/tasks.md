## 1. Adapter core (`internal/ingest/sources/eures.go`)

- [x] 1.1 Search request/response types and the paginated search loop: `POST
      .../jv-search/search` with `locationCodes=[board]`, `occupationUris=[C25, C35]`,
      `publicationPeriod=LAST_THREE_DAYS`, `sortSearch=MOST_RECENT`, `resultsPerPage=50`,
      advancing `page` until a short page, `numberRecords` is reached, or the depth-cap page
      backstop (`10000 / resultsPerPage`) is hit — never issuing a request past the cap.
- [x] 1.2 Detail request/response types and per-posting detail fetch (`GET .../jv/id/{id}
      ?requestLang=en`) via the existing `fetchDetails`/`defaultDetailWorkers` helper, used only
      to resolve `Location` from `jvProfiles[preferredLanguage].locations[]`
      (`cityName`/`addressLines`/`region`/`countryCode`); a failed fetch or a profile with no
      usable place data falls back to the board's own country name rather than dropping the
      posting.
- [x] 1.3 Posting → `Job` mapping: `ExternalID`=`id`, `URL`=EURES portal detail page
      (`https://europa.eu/eures/portal/jv-se/jv-details/{id}?lang=en`), `Title`=search result's
      `title`, `Company`=`employer.name`, `Description`=`sanitizeHTML(description)`,
      `PostedAt` from `creationDate` (unix ms), `Countries` from `locationMap` keys via
      `location.NormalizeCountry`, `EmploymentType` from `positionOfferingCode`
      (`internship`/`contract`) falling back to `positionScheduleCodes`
      (`fulltime`→`full_time`/`parttime`→`part_time`), `IsTechHint=true` unconditionally.
- [x] 1.4 Small internal country-code → display-name table for the ~31 EURES-covered countries,
      used to build `Location` when no city/address text is available.
- [x] 1.5 Implement the `aggregator` marker interface on the adapter type.
- [x] 1.6 `NewEures` constructor + `Provider() string { return "eures" }`.

## 2. Registration (`internal/ingest/sources/registry.go`)

- [x] 2.1 Add `NewEures(c)` to `sources.All`'s keyless multi-company aggregator section,
      alongside `NewArbeitsagentur(c)`/`NewTrudvsem(...)`.

## 3. Tests (`internal/ingest/sources/eures_test.go`)

- [x] 3.1 Table-driven test over a fake HTTP client covering: pagination continuing across pages,
      stopping on a short page, stopping on reaching `numberRecords`, and the depth-cap backstop
      never issuing a request past the cap.
- [x] 3.2 Request-body assertions: `locationCodes` carries the board, `occupationUris` carries
      both ISCO group URIs, `publicationPeriod` is `LAST_THREE_DAYS`, `sortSearch` is
      `MOST_RECENT`.
- [x] 3.3 Field-mapping tests: full happy path (search + successful detail with `cityName`),
      detail fetch failure falls back to country-name `Location` without dropping the posting,
      `EmploymentType` mapping from both `positionOfferingCode` and `positionScheduleCodes`
      (including the "neither maps" empty case), `Countries` via `NormalizeCountry`,
      `IsTechHint=true` on every yielded job, `URL` built from the portal pattern regardless of
      `applicationInstructions` content.
- [x] 3.4 Marker test: `eures` appears in `AggregatorProviders`, does NOT appear in
      `BoardKeyedProviders`'s complement (i.e. it IS board-keyed) or in `FullCatalogProviders`.

## 4. Verification

- [x] 4.1 `gofmt -l` the new/changed Go files (must print nothing).
- [x] 4.2 `go build ./... && go vet ./...`.
- [x] 4.3 `go test ./internal/ingest/sources/...` green, including the new tests from section 3.
- [x] 4.4 `make gen-contracts` and commit the regenerated `web/src/lib/generated/contracts.ts`
      (required whenever a new provider key is registered — `sources.FilterableProviders()`
      feeds the source facet's generated value list).
