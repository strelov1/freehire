## 1. Search mapping & first-party filter

- [x] 1.1 Add `internal/sources/arbeitsagentur_test.go` with an inline search-response fixture (real
  shape: `stellenangebote` with `refnr`, `titel`, `arbeitgeber`, `arbeitsort{ort,region,land}`,
  `aktuelleVeroeffentlichungsdatum`, some with `externeUrl`) and a RED test asserting that a fake
  search+detail client yields a `Job` per first-party posting with the mapped fields, and that
  `externeUrl` postings are dropped.
- [x] 1.2 Create `internal/sources/arbeitsagentur.go`: the `arbeitsagentur` struct over a client
  interface combining `GetJSONWithHeaders` (search) and `GetHTML` (detail), the `arbeitsagenturJob`
  search-result struct, the `X-API-Key` constant, the `berufsfeld`/`veroeffentlichtseit`/`size`/`page`
  query builder, and a `toJob` mapper (refnr→ExternalID, jobdetail URL, titel, arbeitgeber, arbeitsort
  location, publish date) that drops `externeUrl` postings. Make 1.1 green.

## 2. Detail-page description

- [x] 2.1 RED test: given a saved real jobdetail HTML fixture under `testdata/`, the adapter extracts
  the `Stellenbeschreibung` (meta-description fallback) into the sanitized `Job.Description`; a detail
  fetch error or a page with no description yields the `Job` with an empty description without aborting.
- [x] 2.2 Implement the detail fetch via `fetchDetails(kept, defaultDetailWorkers, ...)` over
  `GetHTML(jobdetailURL)` and the description extractor. Make 2.1 green.

## 3. Pagination

- [x] 3.1 RED test: a fake search client returning two full pages then a short page is paginated until
  exhausted (assert the exact set of requested `page` values and the total kept jobs); the depth cap is
  respected.
- [x] 3.2 Implement the `page=1..N` loop (stop on short page / `maxErgebnisse` / depth cap). Make 3.1 green.

## 4. Classification & registration

- [x] 4.1 Add `Provider()` returning `"arbeitsagentur"` (no boardless/aggregator marker); register
  `arbeitsagentur` in `sources.All` unconditionally (public constant key). Test the provider resolves
  from `All(nil)` and is in `FilterableProviders()`. Verify `go build ./... && go vet ./...`.

## 5. Board file & docs

- [x] 5.1 Add `sources/arbeitsagentur.yml` with one entry per IT `berufsfeld` (`Informatik`,
  `Softwareentwicklung und Programmierung`, `IT-Netzwerktechnik, -Administration, -Organisation`,
  `IT-Systemanalyse, -Anwendungsberatung und -Vertrieb`), `board` = the field label; confirm
  `go run ./cmd/ingest sources/arbeitsagentur.yml` validates against the registry (fail-fast passes).
- [x] 5.2 No-op: `internal/sources/AGENTS.md` does not enumerate adapters by class.

## 6. Verify end-to-end

- [x] 6.1 Run `go test ./internal/sources/...` (all green), then a throwaway live smoke crawl against
  the real API for one berufsfeld to confirm first-party postings map, descriptions scrape, and
  externeUrl re-lists drop without error (not committed).
