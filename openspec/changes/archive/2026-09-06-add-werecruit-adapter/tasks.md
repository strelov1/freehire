## 1. Adapter core (`internal/ingest/sources`)

- [x] 1.1 Board validation: parse `<locale>/<tenant>`, requiring both non-empty segments; reject
      anything else with an error before any request is issued.
- [x] 1.2 Listing fetch + extraction: GET `https://careers.werecruit.io/<locale>/<tenant>`,
      locate the `window.allOffers = ` assignment, and decode the JSON array that follows via a
      `json.Decoder` positioned right after the marker (not a hand-rolled end-of-array regex).
- [x] 1.3 Per-posting description hydration: for each listed offer, GET its own `Url` and extract
      the `description` block's inner HTML (reusing `elementInnerHTMLByClass`), fanned out under
      the shared bounded-concurrency `fetchDetails` helper — mirroring `factorial.go`'s shape
      (fetch every detail every crawl, not a `HydratingSource`).
- [x] 1.4 Posting → `Job` mapping: external id from `Id`, `TitleTranslated`, location from
      `Address_City`/`Address_Region`, `Countries` from `Address_State` (a country code),
      `EmploymentType` from `TimeTranslated` ("Full time"/"Part time" only), `PostedAt` from
      `PublicationStartDate`, `URL` from the listing's own `Url`, sanitized description from the
      detail fetch, `Company` from the board's `CompanyEntry`.
- [x] 1.5 Register `NewWerecruit` under provider key `werecruit` in `registry.go`.

## 2. Public URL recognition (`internal/ingest/atsboard`)

- [x] 2.1 Add a new `modePathPair` extraction mode (board = the first two path segments,
      verbatim — no locale folding, unlike `modePathLocalePair`) and a
      `{"careers.werecruit.io", "werecruit", modePathPair}` row in `atsBoards`.
- [x] 2.2 Add `TestRecognize` cases: a well-formed `careers.werecruit.io/<locale>/<tenant>/offers/<slug>`
      link resolves to `(werecruit, "<locale>/<tenant>")`; a URL with only a locale segment or no
      path is not recognized.

## 3. Documentation

- [x] 3.1 Add a "werecruit traps" section to `internal/ingest/sources/AGENTS.md`: the embedded
      `window.allOffers` mechanism and why it needs no pagination, the locale-load-bearing trap
      (an unconfigured locale answers empty, not an error) and why unioning locales is
      unnecessary here (unlike Dayforce), the `Address_State`-is-a-country-code note, and the
      non-`HydratingSource` rationale.

## 4. Verification

- [x] 4.1 `gofmt -l` the changed/new Go files (must print nothing).
- [x] 4.2 `go build ./... && go vet ./...`.
- [x] 4.3 `go test ./internal/ingest/sources/... ./internal/ingest/atsboard/...` green, including
      new tests for tasks 1.1-1.4 and 2.2.
- [x] 4.4 `make gen-contracts` and commit the regenerated `web/src/lib/generated/contracts.ts`.
