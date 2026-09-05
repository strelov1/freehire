## 1. The `fullBoardListing` marker

- [x] 1.1 Add the `fullBoardListing` marker interface to `internal/ingest/sources/source.go`,
      beside `selfClosing`/`fullCatalog`/`sweepGrace`. Comment states the bar: an implementer's
      `Fetch` must structurally prove, for every board it crawls, that it retrieved the board's
      full posting list, and must hard-fail rather than return a partial result as success — see
      `design.md`'s "The bar for earning the marker" and `habrcareer.go`'s existing pattern for
      the unrelated `fullCatalog` marker.
- [x] 1.2 Add `FullBoardListingProviders() map[string]bool` to `registry.go`, built the same way
      as `SelfClosingProviders`/`FullCatalogProviders` (a type-assertion walk over `All()`).
- [x] 1.3 Unit test: a fake adapter implementing the marker appears in
      `FullBoardListingProviders()`; one that doesn't, does not.

## 2. The query

No migration. The predicate rides the existing `(source, external_id text_pattern_ops)` index
through `externalid.BoardPattern`, the same pattern the seen-set and `BoardTracked` already use.

- [x] 2.1 Add `CloseUnseenJobsForBoard :one` to `internal/platform/db/queries/jobs.sql`, beside
      `CloseUnseenJobs`: identical statement with `company_slug = ANY(...)` swapped for
      `external_id LIKE @board_pattern`. Copy the `search_delete_outbox` CTE verbatim — without it
      the sweep closes rows in Postgres and leaves them in the search index until the next full
      rebuild. Comment states why the board scope exists and why it requires the
      `fullBoardListing` marker (see `design.md`).
- [x] 2.2 `make sqlc`; commit the regenerated `internal/platform/db` in the same commit.
- [x] 2.3 Integration test in `internal/platform/db`: the board's stale open rows close; a
      recently-seen row of the same board survives; another board of the same provider is
      untouched; a board whose id is a prefix of another's is not caught (the `:` terminator in
      `BoardPattern` is what prevents it — pin it); a closed row lands in `search_delete_outbox`.

## 3. Which boards qualify

- [x] 3.1 In `internal/ingest/pipeline`, record per board whether the run PROVED it covered that
      board: crawl did not fail (`Failed == 0` for that board, checked independently of
      `recordSuccess`'s health verdict — see design.md's "The `Failed>0` refinement is
      load-bearing"), and the crawl reached postings (`boardReachedPostings`:
      `Ingested + Rejected + ATSCovered > 0`; `Skipped` deliberately excluded).
- [x] 3.2 Unit test: `TestRunReportsNoBoardWhenAStreamDiedMidCrawl` (or equivalent) — a stream that
      fails partway through after partial progress must NOT qualify its board, even though
      `recordSuccess` treats the run as healthy.
- [x] 3.3 Carry the qualifying boards out on the per-provider `Stats` the Runner already returns.
      Do not widen the `BoardHealth` port (see design.md's rationale).
- [x] 3.4 A boardless entry (`board == ""`) never qualifies — `BoardPattern("")` matches the
      provider's whole catalogue. Test this explicitly.

## 4. The sweep

- [x] 4.1 In `cmd/ingest`, after the existing company-scoped close, run a board-scoped close per
      qualifying board, gated on all four conditions: crawl proof (section 3), the entry names a
      board, the provider is not `sweepGrace`/self-closing/`fullCatalog`, and the provider is in
      `FullBoardListingProviders()`. Keep the company-scoped close exactly as it is.
- [x] 4.2 Log each board-scoped close with its board and count.
- [x] 4.3 A per-board failure is logged and the sweep continues to the next board; the run still
      exits non-zero, matching how a provider-level sweep failure is already handled.

## 5. Tests — what must NOT close

Unit-level, against the pipeline/sweep fakes.

- [x] 5.1 A board that yielded zero postings closes nothing, though its crawl succeeded.
- [x] 5.2 A board whose crawl failed (including a mid-crawl failure after partial progress)
      closes nothing.
- [x] 5.3 A boardless entry closes nothing through the board scope.
- [x] 5.4 A provider declaring `sweepGrace` closes nothing through the board scope.
- [x] 5.5 A board the run never reached closes nothing.
- [x] 5.6 A provider whose adapter does NOT carry `fullBoardListing` closes nothing through the
      board scope, even when every other condition holds.
- [x] 5.7 **The leak, closed:** a company the run wrote NO posting for, whose stale job sits on a
      board that yielded, on a provider carrying `fullBoardListing`, IS closed.
- [x] 5.8 A board that yielded only REJECTED postings still qualifies — the crawl reached them.
- [x] 5.9 `go build ./... && go vet ./... && go test ./...`, then `go vet -tags=integration ./...`
      and the tagged suites for `internal/platform/db`, `cmd/ingest`, `internal/ingest/pipeline`.

## 6. Phase-1 adapter audit

- [ ] 6.1 Re-measure current stale-row volume per provider (the figures in the old issue thread
      are from 2026-09-02) to confirm audit priority order. **Not done this session — no
      production database access from this environment.** Re-run before/soon after deploy to
      confirm the candidate list below was still the right priority order.
- [x] 6.2 Audit the large ATS platform adapters against the structural bar (candidates: `ukg`,
      `workday`, `paylocity`, `careerplug`, `greenhouse`, `lever`, `ashby`, `workable`,
      `smartrecruiters`, `personio`, `recruitee`, `bamboohr`, `jazzhr`, `icims`,
      `successfactors`, `jobvite`, `breezyhr` (registered as `breezy`), `eightfold`, `taleo`).
      For each: read `Fetch`, determine pass / needs-hardening / defer. Result: all 19 pass
      (13 with no code change: `ukg`, `paylocity`, `greenhouse`, `lever`, `ashby`, `workable`,
      `smartrecruiters`, `personio`, `recruitee`, `bamboohr`, `jazzhr`, `successfactors`,
      `jobvite`, `breezy`, `eightfold` — each either a single unpaginated request or an
      authoritative-total loop with no artificial cap; 3 needed hardening: `workday`,
      `careerplug`, `icims`, `taleo`). None deferred.
- [x] 6.3 For each adapter that needs hardening to pass (e.g., a soft page cap becomes a hard
      error, or a total-count check is added), make that change and confirm it against the
      adapter's existing tests plus any new ones the hardening needs. Hardened: `workday`
      (`splitByFacet`'s two silent-partial-success fallbacks now fail instead), `careerplug`
      (switched `crawlPagedLinks` → `crawlAllPagedLinks`), `icims` (`jobLocs` no longer skips a
      failed sub-sitemap silently), `taleo` (`listRequisitions`' page-cap exhaustion now fails
      instead of returning a truncated result).
- [x] 6.4 Implement the `fullBoardListing` marker on each adapter that passes; leave the rest
      unmarked and record which were deferred and why. All 19 candidates pass; none deferred.
- [x] 6.5 Confirm `solidjobs` remains unmarked (negative control — it must not pass the bar as-is).
      Pinned by `TestSolidJobsIsNotFullBoardListing`.

## 7. Documentation

- [x] 7.1 `docs/agents/job-lifecycle.md`: mechanism (1) gains the board scope, gated on the
      `fullBoardListing` marker. State plainly that the company-scope leak remains accepted for
      any provider without the marker.
- [x] 7.2 `internal/ingest/pipeline/AGENTS.md`: the board-qualification rule, the `Failed>0`
      refinement, and why zero-yield is refused.
- [x] 7.3 `internal/ingest/sources` package docs (or `source.go`'s own comments): the
      `fullBoardListing` bar, referencing `habrcareer.go` as the model and `solidjobs.go` as the
      negative example that motivated it.

## 8. Verify on prod

- [ ] 8.1 After deploy, read the per-board close lines over the first fleet cycle for each
      Phase-1-marked provider. A single board closing an outsized share of a provider's total is
      the signal to stop and re-ingest that board.
- [ ] 8.2 Confirm `company_slug=pipe`'s 2013 `trakstar` row closes once that board next runs and
      yields, if `trakstar` is among the Phase-1 providers — the worked example from
      freehire#2328. If not, note that it remains open by design pending a later audit wave.
- [ ] 8.3 Watch `search_delete_outbox` depth and drain pace against the new wave of closes.
