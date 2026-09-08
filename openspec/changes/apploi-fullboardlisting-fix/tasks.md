## 1. Investigate before touching code

- [x] 1.1 Read `apploi.go` end to end: confirmed its "short page" proof is already a valid
      natural-end signal, and that it has no cross-page dedup and no per-posting detail fetch
      (so neither the dedup-vs-raw-count issue nor the `unreadableDetail` gap applies here).
- [x] 1.2 Confirm the public `api.apploi.com/v1/jobs` endpoint is reachable without auth (a
      plain `curl` against a real employer id succeeded).
- [x] 1.3 Measure real board sizes on prod before deciding whether `apploiMaxPages` (10,000
      postings/board) needed raising: total open `apploi` postings (1,473,738) and active
      boards (5,833) via cheap `count(*)` queries; a `GROUP BY company_slug` full scan was
      attempted, found still running after 8-10 minutes in `pg_stat_activity`, and cancelled
      via `pg_cancel_backend` (twice) rather than left running. A `TABLESAMPLE SYSTEM (10)`
      pass completed quickly and found the largest company-level aggregate at ~8,700 — an
      upper bound on any single board, comfortably under the cap.

## 2. Fix the adapter

- [x] 2.1 `Fetch`'s pagination loop: a later page failing to fetch returns a hard error instead
      of breaking with a partial result.
- [x] 2.2 Reaching `apploiMaxPages` without ever seeing a page shorter than `apploiPageSize`
      returns a hard error instead of silently returning what was gathered.
- [x] 2.3 Add `func (apploi) fullBoardListing() {}`.

## 3. Tests

- [x] 3.1 `TestApploiFetchFailsOnALaterPageError`: a page-2 failure now fails `Fetch`.
- [x] 3.2 `TestApploiFetchFailsWhenListingExceedsThePageCap`: an "endless" fake `JSONGetter`
      (mirroring `taleoEndlessFake`/`gustoEndlessFake`) serves a full-size page for every
      offset, forcing the walk through the whole cap; asserts failure and exactly
      `apploiMaxPages` calls, not more.
- [x] 3.3 `TestApploiRegisteredAsFullBoardListing`: the same `FullBoardListingProviders(All(nil))`
      one-liner every other marked provider's test file carries.
- [x] 3.4 Confirm every pre-existing `apploi` test still passes unchanged.

## 4. Documentation

- [x] 4.1 Update `internal/ingest/sources/AGENTS.md`: add `apploi` to the marked-provider list.

## 5. Wrap-up

- [x] 5.1 `gofmt -l .`, `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`,
      `go test ./...` all clean.
