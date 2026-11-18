## 1. Source adapter

- [x] 1.1 Add `internal/ingest/sources/firefish.go`: `NewFirefish(c XMLGetter) Source`
      constructor, `Provider() string` returning `"firefish"`, marked `fullBoardListing`,
      implementing `HydratingSource` (`FetchNew` delegating to `Fetch`).
- [x] 1.2 Map the RSS document against a verbatim fixture captured from
      `careers.arnoldclark.com` (2026-11-07): identity from `ffAdvert:ReferenceNumber`, URL
      from the item link, body from CDATA `description` through `sanitizeHTML`, facets from
      the `ffAdvert` elements.
- [x] 1.3 Map structured salary only when the platform states a recognised `PaymentRate` and a
      positive `MinimumPayment` (0.00 = not stated); drop it whole otherwise.
- [x] 1.4 Map `JobType` onto the employment-type vocabulary via an explicit map, unknown →
      unset; `Country` through `location.NormalizeCountry`; `ClosingDate` to
      `ApplicationDeadline` bypassing NotFuture.
- [x] 1.5 Drop items without a reference number or a link; dedup by reference; read a
      well-formed item-less channel as a successful empty board; treat a non-RSS answer as a
      failure and a 404/410 as `ErrBoardGone`.

## 2. Registration

- [x] 2.1 Register `firefish` in `internal/ingest/sources/registry.go`'s `All(c)` as a plain
      `XMLGetter` entry (no pacing measured or needed: one GET per board per run).
- [x] 2.2 Regenerate `web/src/lib/generated/contracts.ts` (`go run ./cmd/gen-contracts`) so the
      source facet lists `firefish`.

## 3. Board recognition

- [x] 3.1 Add the `{"firefishsoftware.com", "firefish", modeHost}` row: every path on a tenant
      subdomain resolves to `(firefish, <tenant host>)`, collapsing to the bare host.
- [x] 3.2 Decline the vendor's own hosts (`www` marketing, `jobs` careers site) through
      `platformLabelsByApex`.
- [x] 3.3 Test cases in `internal/ingest/atsboard/board_test.go`: the tenant career site, its
      RSS feed and a vacancy page all recognize to one board; `www` and `jobs` are declined.

## 4. Verification

- [x] 4.1 Live check by hand (`go run` of a throwaway cmd, since removed): 342 postings on
      `careers.arnoldclark.com` with correct titles, locations, gb countries, GBP/year salary
      bounds and descriptions; 3 postings on the vendor's own `jobs.firefishsoftware.com`;
      an unknown label fails at DNS.
- [x] 4.2 `go test ./internal/ingest/sources/... ./internal/ingest/atsboard/...` green,
      `go vet` clean, `gofmt -l` clean.
