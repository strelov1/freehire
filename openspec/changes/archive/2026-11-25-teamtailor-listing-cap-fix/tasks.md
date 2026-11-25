## 1. Confirm the live bug before touching code

- [x] 1.1 Query prod (read-only) for real `teamtailor` open-job counts per company, to find the largest real boards to probe.
- [x] 1.2 Live-probe the two largest real boards' listing pages around and past the old `ttMaxPages=100` boundary to confirm which are truncated and which aren't.
- [x] 1.3 Narrow down the truncated board's real end (binary-search a few page numbers) to ground the new ceiling in a measured number, not a guess.

## 2. Fix the adapter

- [x] 2.1 Raise `ttMaxPages` (`internal/ingest/sources/teamtailor.go`) with a doc comment citing the measurement, not a round-number guess.
- [x] 2.2 `jobURLs`: a later-page fetch error returns a hard error instead of `break`ing to a partial success.
- [x] 2.3 `jobURLs`: exhausting the loop without ever finding an empty page returns a hard error, mirroring `taleo.go`'s existing message shape for the same situation.
- [x] 2.4 Add `func (teamtailor) fullBoardListing() {}`.

## 3. Tests

- [x] 3.1 `TestTeamtailorFetchFailsOnALaterPageError`: a page-2 failure now fails `Fetch`.
- [x] 3.2 `TestTeamtailorFetchFailsWhenListingExceedsThePageCap`: an "endless" fake `HTMLGetter` (mirroring `taleo_test.go`'s `taleoEndlessFake`) forces the walk through the whole raised cap; asserts failure and exactly `ttMaxPages` calls, not more.
- [x] 3.3 `TestTeamtailorIsFullBoardListing`: the same `FullBoardListingProviders(All(nil))["teamtailor"]` one-liner every other marked provider's test file already carries.
- [x] 3.4 Confirm every pre-existing `teamtailor` test still passes unchanged (none exercised a later-page failure or the old cap).

## 4. Documentation

- [x] 4.1 Update `internal/ingest/sources/AGENTS.md`'s `fullBoardListing` paragraph: add `teamtailor` to the marked-provider list, and note the audit's actual state (8 hand-rolled adapters investigated, 1 fixed, 7 still open) rather than leaving the stale "nobody has audited it yet" blanket statement.

## 5. Wrap-up

- [x] 5.1 `go vet -tags=integration ./...` and the full test suite for `internal/ingest/sources`.
- [x] 5.2 `gofmt -l .`, `go vet ./...`, `go test ./...` clean before commit.
