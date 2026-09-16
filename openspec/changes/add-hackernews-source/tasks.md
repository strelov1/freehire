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

## 3b. Multi-agent code review (post-adoption)

- [x] Ran `/code-review` against the full diff. 9 findings came back; verified each against
      the codebase before acting (receiving-code-review discipline). Fixed by severity:
  - **WorkMode contract violation (most severe, confirmed via source.go's doc comment and
    jobderive.go's precedence chain)**: `Job.WorkMode` was set from `isRemote()` scanning the
    entire joined header tail (title/commitment/salary, not just Location) — `source.go`
    documents WorkMode as carrying only a platform-STRUCTURED signal, never a free-text/
    location heuristic, and `jobderive.go:169` gives a set `WorkMode` precedence over its own
    location/description dictionary. Fixed: `WorkMode` is no longer set by this adapter at
    all; `Job.Remote` (not contract-restricted) is left as-is. Verified live: after the fix,
    `cmd/ingest hackernews` against the real thread now shows a proper
    remote/hybrid/onsite/empty split (153/46/54/161) via the pipeline's own dictionary,
    instead of the adapter's crude remote-or-nothing guess.
  - **hackernewsURL regex swallowed trailing punctuation** (`https?://\S+` ate a closing
    paren/bracket right after a URL with no space, corrupting the stripped title). Fixed:
    stops before `)`, `]`, `}`, `>`, `,`.
  - **Company (parts[0]) wasn't URL-stripped** unlike Title and Location, so a header
    leading with a bare link stored that URL as the company. Fixed: same stripping as
    Title/Location, and an all-URL employer segment is now dropped (empty after stripping),
    matching the existing all-URL-title drop.
  - **hackernewsText duplicated `textFromHTML`** (japandev.go, same package, same
    html.Parse+textContent shape). Fixed: removed the duplicate, reused `textFromHTML`.
  - **Sequential thread fetches** doubled the crawl's wall-clock latency for two independent
    requests. Fixed: fetch concurrently (`sync.WaitGroup`, index-addressed results — no
    shared mutable state, verified race-free with `go test -race`); any single failure still
    fails the whole crawl per the `fullCatalog` contract.
  - **Redundant double `NotFuture`** (`parseRFC3339` already applies it internally). Fixed:
    removed the outer wrap.
  - **Doc comment overstated the role-first-header guard's coverage** (claimed any role-first
    header is dropped; the actual guard only fires on a seniority-word+role-noun pair, a
    deliberate conservative design already explained lower in the same file to avoid
    false-positive drops of real employers like "Lead Bank"). Fixed: corrected the doc
    comment to describe the actual, narrower, deliberate scope rather than widening the
    heuristic under review-fix pressure — broadening it risks new false-positive employer
    drops and deserves its own considered change, not a rushed one here.
  - **threads() found-fewer-than-2 case had no signal.** Not treated as an error (finding
    only 1 is legitimate — e.g. right after a fresh deploy, before the new month's thread has
    posted), since the code's own stated design already accepts variable thread coverage as
    normal `fullCatalog` behavior. Fixed: added a log line so reduced coverage is visible to
    an operator instead of silent.
  - **Skipped, stated why**: the finding that `toJob()` HTML-parses the same comment text up
    to three times (header/link/description) is real but efficiency-only, not correctness —
    fixing it well needs restructuring `hackernewsParseHeader`'s signature (a public,
    directly-unit-tested function) to share one parsed tree, which is a bigger, riskier
    change than the bug fixes above for a non-broken outcome. Left as a documented follow-up
    rather than rushed here.
- [x] Re-ran the full verification suite (3.1-3.5) after the fixes — all still pass; the live
      `cmd/ingest hackernews` re-run against `hire-db-1` still shows 414 jobs, now with the
      corrected remote/hybrid/onsite split described above.

## 4. Follow-up (not part of this change's diff)

- [ ] 4.1 Note in the PR description: board-catalog promotion for HN-mentioned companies
      with a recognizable ATS link is `python3 scripts/harvest_boards.py --hn --write` (fixed
      by PR #2879), run as a periodic operational task — same shape as
      harvest-githublists-boards' deferred group 3, not scheduled here.
