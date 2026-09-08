## 1. hh (audited, excluded)

- [x] 1.1 Live-verify hh.ru's own search-depth behavior against a currently-configured board
      (professional_role 96) before deciding whether hh qualifies.
- [x] 1.2 Revert `crawl`'s cap-exhaustion behavior to the original soft return (no hard failure);
      keep the later-page fetch/decode failure as a hard error (unrelated to the depth-cap issue).
- [x] 1.3 Do NOT add the `fullBoardListing` marker; document why in `hhru.go`'s own comments.
- [x] 1.4 Tests: replace the marker-registration and cap-exhaustion tests with ones asserting hh
      does NOT implement the marker and that exhausting the cap is a plain success.

## 2. neogov

- [x] 2.1 `Fetch`'s listing loop: a later-page fetch failure returns a hard error; the empty-page
      proof reads the RAW per-page item count (before cross-page dedup), not the count of newly
      added items; reaching `neogovMaxPages` without either that or the stated total being reached
      returns a hard error.
- [x] 2.2 Add the `fullBoardListing` marker method.
- [x] 2.3 Tests: page-cap-exhaustion regression (`neogovEndlessFake`), marker-registration, and a
      duplicate-only-page regression (`neogovDuplicateThenNewFake`) proving a non-empty page of
      only-already-seen postings does not end the walk early.

## 3. edjoin

- [x] 3.1 `list`: every page failure (first or later) returns a hard error; the empty-page proof
      reads the raw `resp.Data` row count, not the count of newly-kept rows; reaching
      `edjoinMaxPages` without either that or `totalRecords` being reached returns a hard error.
- [x] 3.2 Add the `fullBoardListing` marker method.
- [x] 3.3 Tests: update `TestEdjoinListPageFailures` for the new later-page behavior; add
      page-cap-exhaustion regression (`edjoinEndlessFake`), marker-registration, and a
      duplicate-only-page regression (`edjoinDuplicateThenNewFake`).

## 4. workstream

- [x] 4.1 `workstreamTotalPages` reports whether its page count was genuinely stated
      (`stated bool`) rather than only a page number, so the caller can tell a declared-count proof
      apart from the page-cap fallback.
- [x] 4.2 `list`: a later-page fetch failure returns a hard error; the empty-page proof (used only
      when no valid stated total-pages count exists) reads the raw per-page card count, not the
      count of newly-kept cards; reaching `workstreamMaxPages` without either proof returns a hard
      error.
- [x] 4.3 Add the `fullBoardListing` marker method.
- [x] 4.4 Tests: page-cap-exhaustion regression (`workstreamEndlessFake`, unstated-total case),
      marker-registration, and a duplicate-only-page regression
      (`workstreamDuplicateThenNewFake`).

## 5. peopleforce

- [x] 5.1 `Fetch`'s listing loop: every page failure returns a hard error; the empty-page proof
      reads the raw per-page card count, not the count of newly-kept cards; reaching
      `peopleforceMaxPages` without it returns a hard error.
- [x] 5.2 `detail()` returns the established `unreadableDetail` marker (matching careerplug.go's
      pattern) on a failed-but-not-404/410 fetch, instead of dropping the posting — peopleforce has
      no `HydratingSource` fallback, so every crawl re-fetches every listed posting's detail.
- [x] 5.3 Add the `fullBoardListing` marker method.
- [x] 5.4 Tests: page-cap-exhaustion regression (`peopleforceEndlessFake`), marker-registration, a
      duplicate-only-page regression, and an unreadable-vs-gone detail regression
      (`TestPeopleForceUnreadableDetailIsMarkedNotDropped` /
      `TestPeopleForceGoneDetailDropsThePosting`).

## 6. gusto

- [x] 6.1 `list`: every page failure returns a hard error; the empty-page proof reads the raw
      per-page card count, not the count of newly-kept cards; reaching `gustoMaxPages` without it
      returns a hard error.
- [x] 6.2 Add the `fullBoardListing` marker method.
- [x] 6.3 Tests: rewrite `TestGustoFetchLaterPageFailureKeepsWhatWasGathered` as
      `TestGustoFetchFailsOnALaterPageError`; add page-cap-exhaustion regression
      (`gustoEndlessFake`), marker-registration, and a duplicate-only-page regression
      (`gustoDuplicateThenNewFake`).

## 7. Docs and verification

- [x] 7.1 Update `internal/ingest/sources/AGENTS.md`: note that five of the eight hand-rolled
      adapters no longer follow the soft "later page ends the walk" rule and now prove emptiness
      from the raw page count; add them to the `fullBoardListing` wave list; document hh's audited
      exclusion with its live measurement; note `bayt`/`teamtailor` remain open.
- [x] 7.2 `gofmt -l`, `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`,
      `go test ./...` all clean.
