## 1. Source adapter skeleton

- [ ] 1.1 Add `internal/ingest/sources/livehire.go`: `NewLiveHire(c livehireHTTP) Source`
      constructor, `Provider() string` returning `"livehire"`, implementing `HydratingSource`.
- [ ] 1.2 Write failing unit test for the listing mapping (segment, urlCode, advertUrlCode,
      facets) against an `httptest`-style fake, then implement the mapping to pass it.

## 2. Listing and pagination

- [ ] 2.1 Write failing test: a board whose listing reports `hasMoreResults=true` followed by a
      final page yields postings from both pages.
- [ ] 2.2 Implement the pagination walk to pass it.
- [ ] 2.3 Write failing test: a later page that re-serves already-seen postings does not
      duplicate them and ends the walk (the endpoint's verified re-serving behaviour).
- [ ] 2.4 Implement cross-page dedup by external id to pass it.

## 3. Listing-stage filters

- [ ] 3.1 Write failing test: a posting missing urlCode or advertUrlCode, and a posting marked
      `internalOnly`, are excluded from the returned postings.
- [ ] 3.2 Implement the listing-stage filters to pass it.

## 4. Per-posting hydration

- [ ] 4.1 Write failing test: `FetchNew` calls the detail endpoint only for ids the `seen`
      callback reports unseen, and marks seen ones `SeenRefresh = true` without a detail call.
- [ ] 4.2 Implement hydration via the shared `fetchDetails` worker-pool helper (per `getro.go`'s
      shape) to pass it.
- [ ] 4.3 Write failing test: a posting whose detail fetch errors is ingested list-only without
      aborting the rest of the board's hydration.
- [ ] 4.4 Implement per-posting error isolation to pass it.

## 5. Posting normalization

- [ ] 5.1 Write failing test mapping a full detail fixture to `Job`: `ExternalID` from
      `<segment>_<urlCode>_<advertUrlCode>`, canonical `URL` built as
      `https://www.livehire.com/careers/<segment>/job/<urlCode>/<advertUrlCode>/<slug>`,
      `Title`, `Location`, `Countries` from the ISO code, `WorkMode` from
      `flexibleWorkOption`.
- [ ] 5.2 Implement the base mapping to pass it.
- [ ] 5.3 Write failing test: `descriptionHtml` maps through `sanitizeHTML` into
      `Description`; `workType` maps onto `EmploymentType` via the enum map; an unrecognized
      value leaves it unset rather than erroring.
- [ ] 5.4 Implement the detail mapping to pass it.

## 6. Salary and closure

- [ ] 6.1 Write failing test: `SalaryMin/Max/Currency` are set only when the detail states both
      bounds and a currency, and left unset otherwise.
- [ ] 6.2 Implement the salary mapping to pass it.
- [ ] 6.3 Write failing test: a posting whose detail marks `applicationsClosed` or a non-null
      `closedAt` is marked `Removed`.
- [ ] 6.4 Implement the closure mapping to pass it.

## 7. Registration

- [ ] 7.1 Register `livehire` in `internal/ingest/sources/registry.go`'s `All(c)` as a plain
      JSONGetter+JSONPoster entry (no special transport).

## 8. Board recognition

- [ ] 8.1 Write failing tests in `internal/ingest/atsboard/board_test.go`: the board listing,
      `/jobs`, and the posting shape on `www.livehire.com` recognize as
      `(livehire, <segment>)`, collapsing to `https://www.livehire.com/careers/<segment>`.
- [ ] 8.2 Write failing tests: `/careers` (no segment), `/pricing`, `/careers-api/...`, and a
      non-slug segment are NOT recognized.
- [ ] 8.3 Implement the `modeKeyedSegment` row + `keyedSegmentBoards` entry to pass both.

## 9. Live verification

- [ ] 9.1 Run the adapter by hand against the live `flybuys` board (`go run ./cmd/ingest
      livehire`, after `cmd/add-board --provider=livehire --board=flybuys --company="Flybuys"`)
      and confirm postings, descriptions, and locations look correct end to end.
- [ ] 9.2 Check `gofmt -l .`, `go vet ./...`, `go test ./...` are clean.
