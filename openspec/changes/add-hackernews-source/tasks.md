## 1. Adopt and review the adapter

- [ ] 1.1 Fetch `Manan-Santoki/freehire@us-aggregator-sources`, extract
      `internal/ingest/sources/hackernews.go` and `hackernews_test.go` from commit
      `1daaa55a` (its base is `09076f3e`; do not take anything else from that commit —
      `githublists`/`hiringcafe` files stay out).
- [ ] 1.2 Place both files in `internal/ingest/sources/`, run `go build ./internal/ingest/sources/...`
      and `go test ./internal/ingest/sources/...` — confirm they compile and pass unmodified
      against current `main` before changing anything (establishes the RED→"already GREEN"
      baseline this adoption starts from, and surfaces any drift from `main` moving since
      `09076f3e`).
- [ ] 1.3 Full read-through review against design.md's checklist: `Source`/`CompanyEntry`
      shape, `routedHTTP` fixture shape, reused helper signatures
      (`sanitizeHTML`/`isRemote`/`NotFuture`/`parseRFC3339`/`textContent`/`walk`/`attr`),
      and line-by-line correctness (regex correctness, edge cases, error messages) — same
      bar as any other adapter PR. Fix anything found; add a test first (RED) for any fix
      that changes behavior, per the normal TDD loop.

## 2. Register and document

- [ ] 2.1 Add `NewHackerNews(c)` to `registry.go`'s `All()`, in the unconditional block (it
      needs only a keyless `JSONGetter`, no fingerprint transport).
- [ ] 2.2 Add the "Hacker News traps" section from the fork's `AGENTS.md` diff to
      `internal/ingest/sources/AGENTS.md` (only that section — not the hiring.cafe/GitHub
      lists sections from the same diff, both already resolved elsewhere). Review its wording
      against the actually-landed code, not the fork's, in case review (1.3) changed anything.
- [ ] 2.3 `make gen-contracts`; commit the regenerated `web/src/lib/generated/contracts.ts`
      diff (never hand-edit — see the `generated_contracts_conflicts` lesson).

## 3. Verify

- [ ] 3.1 `gofmt -l .` clean.
- [ ] 3.2 `CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./...`.
- [ ] 3.3 `CGO_ENABLED=0 go test ./...` — full suite green (the one known unrelated
      pre-existing `cmd/billing-sync` env-dependent failure, if it still reproduces locally,
      is not caused by this change — see harvest-githublists-boards).
- [ ] 3.4 `go vet -tags=integration ./...`.
- [ ] 3.5 Confirm `internal/ingest/sources/registry.go`'s `All()` includes `hackernews` and
      `cmd/ingest hackernews` runs cleanly against the local dev DB (a real Algolia call —
      network-dependent, run once by hand, not part of the automated suite).

## 4. Follow-up (not part of this change's diff)

- [ ] 4.1 Note in the PR description: board-catalog promotion for HN-mentioned companies
      with a recognizable ATS link is `python3 scripts/harvest_boards.py --hn --write` (fixed
      by PR #2879), run as a periodic operational task — same shape as
      harvest-githublists-boards' deferred group 3, not scheduled here.
