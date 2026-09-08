## 1. hh

- [x] 1.1 `crawl`: a later-page fetch or state-decode failure returns a hard error instead of
      breaking with a partial result; reaching `hhMaxPages` without a genuinely empty page also
      returns a hard error.
- [x] 1.2 Add the `fullBoardListing` marker method.
- [x] 1.3 Tests: page-cap-exhaustion regression (`hhEndlessFake`), marker-registration.

## 2. neogov

- [x] 2.1 `Fetch`'s listing loop: a later-page fetch failure returns a hard error; reaching
      `neogovMaxPages` without either a genuinely empty page or the stated total being reached
      returns a hard error.
- [x] 2.2 Add the `fullBoardListing` marker method.
- [x] 2.3 Tests: page-cap-exhaustion regression (`neogovEndlessFake`), marker-registration.

## 3. edjoin

- [x] 3.1 `list`: every page failure (first or later) returns a hard error; reaching
      `edjoinMaxPages` without either a genuinely empty page or `totalRecords` being reached
      returns a hard error.
- [x] 3.2 Add the `fullBoardListing` marker method.
- [x] 3.3 Tests: update `TestEdjoinListPageFailures` for the new later-page behavior; add
      page-cap-exhaustion regression (`edjoinEndlessFake`) and marker-registration tests.

## 4. workstream

- [x] 4.1 `workstreamTotalPages` reports whether its page count was genuinely stated
      (`stated bool`) rather than only a page number, so the caller can tell a declared-count proof
      apart from the page-cap fallback.
- [x] 4.2 `list`: a later-page fetch failure returns a hard error; reaching `workstreamMaxPages`
      without a stated total-pages count or a genuinely empty page returns a hard error.
- [x] 4.3 Add the `fullBoardListing` marker method.
- [x] 4.4 Tests: page-cap-exhaustion regression (`workstreamEndlessFake`, unstated-total case) and
      marker-registration.

## 5. peopleforce

- [x] 5.1 `Fetch`'s listing loop: every page failure returns a hard error; reaching
      `peopleforceMaxPages` without a genuinely empty page returns a hard error.
- [x] 5.2 Add the `fullBoardListing` marker method.
- [x] 5.3 Tests: page-cap-exhaustion regression (`peopleforceEndlessFake`), marker-registration.

## 6. gusto

- [x] 6.1 `list`: every page failure returns a hard error; reaching `gustoMaxPages` without a
      genuinely empty page returns a hard error.
- [x] 6.2 Add the `fullBoardListing` marker method.
- [x] 6.3 Tests: rewrite `TestGustoFetchLaterPageFailureKeepsWhatWasGathered` as
      `TestGustoFetchFailsOnALaterPageError`; add page-cap-exhaustion regression
      (`gustoEndlessFake`) and marker-registration tests.

## 7. Docs and verification

- [x] 7.1 Update `internal/ingest/sources/AGENTS.md`: note that six of the eight hand-rolled
      adapters no longer follow the soft "later page ends the walk" rule, and add them to the
      `fullBoardListing` wave list; note `bayt`/`teamtailor` remain open.
- [x] 7.2 `gofmt -l`, `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`,
      `go test ./...` all clean.
