## 1. Investigate before touching code

- [x] 1.1 Re-read `bayt.go`'s own comments to separate the base TLS-fingerprint block (already
      solved by `fingerprintHTTP`) from the concurrency-specific throttling risk documented for
      the detail fan-out.
- [x] 1.2 Attempt a live check of the listing walk's throttling behavior specifically; confirmed
      this sandbox cannot isolate it (Akamai 403s plain `curl` on TLS-fingerprint grounds
      regardless of pacing) — documented the limitation rather than overclaiming a live
      measurement that wasn't actually made.
- [x] 1.3 Decide, based on the listing walk's own structure (sequential vs. the detail fan-out's
      concurrent pattern), whether excluding `bayt` from the marker remains warranted.

## 2. Fix the adapter

- [x] 2.1 `Fetch`'s listing loop: the empty-page proof reads the raw count of anchors that ARE
      job-detail links (post-`baytJobID` filter, pre-dedup) — not the raw count of every anchor
      on the page, which is never empty due to navigation chrome.
- [x] 2.2 A later listing page failing to fetch, and reaching `baytMaxPages` without a genuinely
      empty page, both return a hard error instead of a partial success.
- [x] 2.3 `detail` gains an `e CompanyEntry` parameter and returns `unreadableDetail` (via
      `detailUnreadable`) on a failed-but-not-404/410 fetch, instead of dropping the posting.
- [x] 2.4 Add `func (bayt) fullBoardListing() {}`.

## 3. Tests

- [x] 3.1 `TestBaytFetchFailsOnALaterPageError`: a page-2 failure now fails `Fetch`.
- [x] 3.2 `TestBaytFetchReachesAPostingPastADuplicateOnlyPage`: a duplicate-only non-empty page
      does not end the walk early.
- [x] 3.3 `TestBaytFetchFailsWhenListingExceedsThePageCap`: an "endless" fake `HTMLGetter`
      (mirroring `taleoEndlessFake`/`gustoEndlessFake`) forces the walk through the whole cap;
      asserts failure and exactly `baytMaxPages` calls, not more.
- [x] 3.4 `TestBaytUnreadableDetailIsMarkedNotDropped` / `TestBaytGoneDetailDropsThePosting`: a
      transient detail failure is marked unreadable, a 404/410 still drops the posting.
- [x] 3.5 `TestBaytRegisteredAsFullBoardListing`: the same `FullBoardListingProviders(All(nil))`
      one-liner every other marked provider's test file carries.
- [x] 3.6 Confirm every pre-existing `bayt` test still passes unchanged.

## 4. Documentation

- [x] 4.1 Update `internal/ingest/sources/AGENTS.md`: move `bayt` from "audited and excluded" to
      the marked-provider list, with the reasoning that resolved the throttling concern.

## 5. Wrap-up

- [x] 5.1 `gofmt -l .`, `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`,
      `go test ./...` all clean.
