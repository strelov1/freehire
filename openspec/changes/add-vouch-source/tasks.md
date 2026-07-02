## 1. Shared flight helper

- [x] 1.1 Extracted the generic Next.js flight primitives into
  `internal/sources/nextflight.go`: `decodeDeelFlight`→`decodeNextFlight`,
  `deelPush`→`nextFlightPush`, and `bracketSlice` (moved verbatim). Updated deel's call
  sites + deel_test.go; deel tests stay green (byte-identical, no behavior change).

## 2. Adapter (TDD, one behavior at a time)

- [x] 2.1 `Provider()` returns `"vouch"`; the adapter is board-based and appears in
  `All()` / `FilterableProviders()` (registration test).
- [x] 2.2 Flight parse — extracts the `listings` array via `bracketSlice`/`decodeNextFlight`;
  a page with no flight / no `listings` returns an error (two tests).
- [x] 2.3 Mapping — each live listing maps to a `Job` (ExternalID = id with empty-id
  drop-guard, URL resolved from listing `url`, Title, Company via
  `firstNonEmpty(company.name, e.Company)`, Description = sanitized pitch+must+nice with
  the plain-text pitch HTML-escaped, Location from `locations`, PostedAt via `parseRFC3339`).
- [x] 2.4 Live-filter — draft / deactivated / unlisted / empty-id listings omitted; only
  `activated && !draft && !unlisted` returned.
- [x] 2.5 Structured work mode — `employmentType` mapped through the shared
  `workplaceTypeMode` with remote>hybrid>onsite priority; `Remote` set when work mode is
  remote or the location heuristic matches.

## 3. Registry + board file

- [x] 3.1 Added `NewVouch(c)` to `sources.All` in `internal/sources/source.go`.
- [x] 3.2 Created `sources/vouch.yml` with the validated Laine entry (board =
  `cmghojtua00p5et0dvusuxx5o`); passes config validation.

## 4. Verify

- [x] 4.1 `go build ./...` / `go vet ./...` / `gofmt -l` clean; full `go test ./...` green
  (including the unchanged deel tests).
- [x] 4.2 Live smoke against the Laine board: returned 11 real live postings with
  populated title/description/location/work-mode/URL in ~1.4s (single fetch, no fan-out);
  throwaway harness discarded.
