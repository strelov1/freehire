## 1. The query

No migration. The predicate rides the existing `(source, external_id text_pattern_ops)` index
through `externalid.BoardPattern`, the same pattern the seen-set and `BoardTracked` already use.

- [x] 1.1 Add `CloseUnseenJobsForBoard :one` to `internal/platform/db/queries/jobs.sql`, beside
      `CloseUnseenJobs`. It is that statement with `company_slug = ANY(...)` swapped for
      `external_id LIKE @board_pattern`. **Copy the `search_delete_outbox` CTE verbatim** —
      without it the sweep closes rows in Postgres and leaves every one of them in the search
      index until the next full rebuild. Comment must say why the board scope exists (the
      company scope cannot retire a company whose last posting left a board we still crawl) and
      why no row-by-row fallback is added (board scope already isolates a corrupted row to one
      board, unlike the provider-wide statement that the 2026-08-11 incident blocked).
- [x] 1.2 `make sqlc`; commit the regenerated `internal/platform/db` in the same commit.
- [x] 1.3 Integration test in `internal/platform/db`: the board's stale open rows close; a
      recently-seen row of the same board survives; another board of the same provider is
      untouched; **a board whose id is a prefix of another's is not caught** (the `:` terminator
      is what prevents it — pin it); a closed row lands in `search_delete_outbox`.

## 2. Which boards qualify

- [x] 2.1 In `internal/ingest/pipeline`, record per board whether the run PROVED it covered that
      board: crawl did not fail, and the crawl reached postings.

      **Corrected while implementing:** this task said `Ingested + Rejected + ATSCovered +
      Skipped > 0`, but the existing predicate it also told me to reuse counts only the first
      three. Reuse won: `Skipped` means the posting was listed and then FAILED TO PERSIST, so a
      board whose every save is failing would prove itself on the strength of the failures.
      `boardReachedPostings` is now the one definition, read by both the streaming path's
      failure test and the board qualification.
- [x] 2.2 Carry the qualifying boards out on the per-provider `Stats` the Runner already
      returns. Do NOT widen the `BoardHealth` port: it answers a board's health, the sweep asks
      about scope, and threading one through the other would make `cmd/ingest` read back through
      the database a fact the run held in memory (see design.md).
- [x] 2.3 A boardless entry (`board == ""`) never qualifies — `BoardPattern("")` matches the
      provider's whole catalogue. Test this explicitly; it is the one condition whose failure is
      catastrophic rather than merely wrong.

## 3. The sweep

- [x] 3.1 In `cmd/ingest`, after the existing company-scoped close, run a board-scoped close per
      qualifying board. Keep the company-scoped close exactly as it is — it reaches boardless
      entries and zero-yield boards, which the board scope does not.
- [x] 3.2 Skip the board scope for a provider that declares `sweepGrace`, is self-closing, or is
      `fullCatalog`. Key on the markers via the existing `sources.*Providers` helpers, never on a
      list of names: `sweepGrace` means "the crawl reaches only a SLICE of the catalogue", which
      is exactly when closing within a board is wrong.
- [x] 3.3 Log each board-scoped close with its board and count. A provider-level number cannot
      distinguish "many boards each retiring a few rows" from "one board mass-closing", and the
      first fleet cycle after deploy is when that distinction matters.
- [x] 3.4 A per-board failure is logged and the sweep continues to the next board; the run still
      exits non-zero, matching how a provider-level sweep failure is already handled.

## 4. Tests

Every case here is about what must NOT close. Unit-level, against the existing pipeline/sweep
fakes.

- [x] 4.1 A board that yielded zero postings closes nothing, though its crawl succeeded.
- [x] 4.2 A board whose crawl failed closes nothing.
- [x] 4.3 A boardless entry closes nothing through the board scope.
- [x] 4.4 A provider declaring `sweepGrace` closes nothing through the board scope.
- [x] 4.5 A board the run never reached closes nothing.
- [x] 4.6 **The leak, closed:** a company the run wrote NO posting for, whose stale job sits on a
      board that yielded, IS closed.
- [x] 4.7 A board that yielded only REJECTED postings still qualifies — the crawl reached them,
      and refusing would spare exactly the non-tech-heavy boards where stale rows accumulate.
- [x] 4.8 `go build ./... && go vet ./... && go test ./...`, then `go vet -tags=integration ./...`
      and the tagged suites for `internal/platform/db`, `cmd/ingest`, `internal/ingest/pipeline`.

## 5. Documentation

- [x] 5.1 `docs/agents/job-lifecycle.md`: mechanism (1) gains the board scope. Rewrite the
      paragraph that documents the company-scope leak as accepted — it no longer is, for boards
      that prove themselves. Keep the leak's description for what remains: boardless entries,
      zero-yield boards, and boards that left `sources/`.
- [x] 5.2 `internal/ingest/pipeline/AGENTS.md`: the board-qualification rule and why zero-yield
      is refused (the Workday `total:0` truncation).
- [x] 5.3 `cmd/liveness`'s `probeDespiteRegistered` comment cites the company-scope leak as its
      reason for existing. It still holds for the boardless aggregators it lists — check the
      wording still reads true and adjust if it over-claims.

## 6. Verify on prod

- [ ] 6.1 After deploy, read the per-board close lines over the first fleet cycle. Expect on the
      order of 215,000 closes in total, spread across boards. A single board closing tens of
      thousands is the signal to stop and re-ingest that board.
- [ ] 6.2 Confirm `company_slug=pipe`'s 2013 trakstar row closes once that board next runs and
      yields — the worked example from freehire#2328. If its board no longer runs at all, it
      stays open by design and belongs to the tail this change does not claim.
- [ ] 6.3 Watch `search_delete_outbox` depth and the drain: 215,000 queued deletions is a larger
      wave than a normal sweep produces, and the drain's pace is what decides how long closed
      rows linger in search.

## 7. Review findings applied

Two parallel reviews (standards, spec) ran against the implementation commit.

- [x] 7.1 **A real correctness hole, found by the spec review.** `sweepableBoard` took
      `recordSuccess`'s decision as "the crawl did not fail". It is not: `ingestStream` sets
      `Failed = 1` on a mid-crawl error AFTER partial progress, and such a board is
      deliberately treated as healthy (a rate-limited stream must not cool a working board).
      So a stream that died at posting 40 of 5,000 qualified its board, and the sweep would
      have closed everything past the point it died — the exact freehire#725 class the design
      calls its load-bearing safety. `st.Failed > 0` is now its own refusal, with a comment
      separating HEALTH from COMPLETENESS. `TestRunReportsNoBoardWhenAStreamDiedMidCrawl`
      reproduces the bug and was confirmed to fail before the fix.
- [x] 7.2 The remaining exposure is recorded rather than solved: an adapter that returns a
      truncated crawl as an unqualified success is invisible here, and always was — the
      company-scoped close has the same hole, which is how #725 happened in the first place.
- [x] 7.3 Standards review: `selfClosing` was a dead parameter — the sweep loop `continue`s on
      it before reaching the board close, so the branch was unreachable and its test asserted
      nothing. Removed.
- [x] 7.4 Standards review: `slices.Compact` after the sort. A board can legitimately appear
      twice (a repeated board-file entry, or one board id recurring across regional slices).
      The duplicate closed nothing extra but doubled the board's log line and the board count.
- [x] 7.5 The "across N boards" log counted boards ASKED, not boards that closed anything, so
      a healthy provider read as if every board had retired rows. It now reports both.
- [x] 7.6 The `fullCatalog` exclusion comment claimed that source "already closes by source
      alone", which is only true on a zero-failure run. Reworded to say what happens in both
      cases, and to record that both such adapters are boardless today so the exclusion is
      belt-and-braces.
- [x] 7.7 Spec review: the `shouldSweep` interaction (a provider whose every posting was
      rejected never reaches either close) was correct but documented nowhere. Now stated in
      `cmd/ingest` and in design.md, with why it is left alone.

## 8. Reverted

- [x] 8.1 Reverted from `main` and production 2026-09-02, eighteen minutes after deploy. The
      first fleet cycle closed 110 live `solidjobs` postings: that adapter reaches only the
      first 500 of the board's ~1,400 and returns them as an unqualified success, so all three
      proof conditions were satisfied honestly and the board still had not been listed.
- [x] 8.2 The 110 rows were reopened and their `search_delete_outbox` entries deleted, so they
      were never removed from the index.
- [x] 8.3 design.md's "What production taught this design" records the constraint any retry
      must satisfy: adapters must OPT IN to being sweepable, because the conditions here test
      what the RUN did and never what the ADAPTER can do.
- [ ] 8.4 Enumerate which adapters can honestly claim they list a board in full. Not started;
      this is the gate on re-attempting.
