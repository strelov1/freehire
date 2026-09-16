## 1. Adopt and review the adapter

- [x] 1.1 Extracted `internal/ingest/sources/hackernews.go` and `hackernews_test.go` from
      `Manan-Santoki/freehire@1daaa55a` (base `09076f3e`) — only those two files.
- [x] 1.2 Compiled and tested unmodified against current `main` first: clean build, all 5
      tests passed with zero drift from the fork's base.
- [x] 1.3 Full review done. Found and fixed one real bug: the salary-segment skip
      (`strings.ContainsAny(seg[:1], "$€£")`) sliced the first BYTE, not rune — € and £ are
      multi-byte in UTF-8, so a segment starting with either was never recognized as a
      salary and was mistaken for the location. Added two RED test cases (`€95k-120k`,
      `£80k-100k` leading a location), then fixed via a new `startsWithCurrencySymbol`
      helper decoding the first rune (`unicode/utf8`). All other aspects (Source/
      CompanyEntry shape, routedHTTP fixture, reused helpers, regex correctness, error
      handling) matched current `main` with no other issues found.

## 2. Register and document

- [x] 2.1 Registered `NewHackerNews(c)` in `registry.go`'s `All()`, same unconditional-block
      insertion point the fork used (still valid on current `main`).
- [x] 2.2 Added the "Hacker News traps" section (only that section) to
      `internal/ingest/sources/AGENTS.md`, plus one extra bullet documenting the
      currency-symbol fix from 1.3 that the fork's own doc didn't know about.
- [x] 2.3 `make gen-contracts` — `SOURCE_VALUES` picked up `hackernews` automatically via
      `FilterableProviders()` reading the registry (boardless+aggregator marker), no manual
      list edit needed (corrects tasks.md's original assumption). Regenerated diff committed.

## 3. Verify

- [x] 3.1 `gofmt -l .` clean.
- [x] 3.2 `CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...` — both clean.
- [x] 3.3 `CGO_ENABLED=0 go test ./...` — full suite green except the same pre-existing,
      unrelated `cmd/billing-sync` env-dependent failure documented in
      harvest-githublists-boards.
- [x] 3.4 `go vet -tags=integration ./...` — clean.
- [x] 3.5 Real end-to-end smoke test against the local dev DB (`hire-db-1`, correct port
      15432 — see the `docker_db_port_collision` memory note this surfaced): `cmd/add-board
      --provider=hackernews --apply` seeds the boardless row, then `cmd/ingest hackernews`
      made a real Algolia call against the live September 2026 thread and ingested **414
      jobs** (2/416 correctly rejected as non-technical). Also had to run every migration
      against `hire-db-1` first — it was stale (missing `jobs.hydrated_at`,
      `board_health.last_yield_at`, etc. from recent migrations), unrelated to this change.
      Spot-checked a random sample of the written jobs: company/title/location/work_mode all
      parsed correctly.

## 4. Follow-up (not part of this change's diff)

- [ ] 4.1 Note in the PR description: board-catalog promotion for HN-mentioned companies
      with a recognizable ATS link is `python3 scripts/harvest_boards.py --hn --write` (fixed
      by PR #2879), run as a periodic operational task — same shape as
      harvest-githublists-boards' deferred group 3, not scheduled here.
