## Context

See proposal.md - Why for how this change arrived at its current (much smaller) shape after
two earlier drafts. Summary of what changed and why, for whoever reads this later:

1. **First draft**: a two-stage crawl→LLM-extraction pipeline modeled on
   `internal/ingest/telegram` (new `hn_posts` table, `cmd/hn-ingest`/`cmd/hn-extract`,
   `Extraction`/`Validate()`, an ATS-link-or-LLM branch per comment). Premise: HN comments
   are free prose needing an LLM, the way a Telegram post can be.
2. **That premise was checked against the contributor's actual fork** and found wrong: the
   thread's own posting convention is a pipe-delimited header, which the fork's
   `internal/ingest/sources/hackernews.go` already parses deterministically — no LLM, no new
   tables, fits the existing `sources.Source`/`pipeline.Runner` crawl shape directly, and
   reuses helpers already in the `sources` package (`sanitizeHTML`, `isRemote`, `NotFuture`,
   `parseRFC3339`, HTML walk/attr helpers).
3. **The remaining piece — promoting a company with a recognized ATS link into the board
   catalog — was checked against `pipeline`/`sources`' own architecture** and found to need
   no new code either: `internal/ingest/sources` and `internal/ingest/pipeline` are
   deliberately DB/board-catalog-unaware (confirmed: neither imports
   `internal/ingest/boardcatalog`; the only callers of `boardcatalog.Insert` are
   `internal/ingest/contribution` and three standalone harvester binaries, never the crawl
   pipeline itself — `internal/ingest/pipeline/AGENTS.md` documents only two patterns for "a
   write beside the job write," a same-transaction `Store` capability or an after-commit
   best-effort outbox, neither of which boardcatalog uses). Wiring board-catalog awareness
   into the shared, heavily-used `pipeline.Runner` for one provider's special case would
   violate that separation for no need: `scripts/harvest_boards.py --hn` already reads the
   same threads for ATS links, and after PR #2879 already writes correct
   `cmd/harvest-boards`-ready seed JSON.

## Goals / Non-Goals

**Goals:**
- Land a correctly-reviewed version of the contributor's adapter, current against `main` (it
  forked at `09076f3e`, main has moved), with the same test coverage.
- Nothing beyond the adapter + registration + docs + generated contracts.

**Non-Goals:**
- No board-contribution code in this change (see Context #3) — that's
  `harvest_boards.py --hn`, already shipped.
- No re-verification of the "hiring.cafe traps" / "GitHub lists traps" sections from the
  same upstream `AGENTS.md` diff — out of scope, already resolved (hiring.cafe rejected,
  githublists fixed as harvest-githublists-boards).

## Decisions

**Adopt, don't re-derive.** The fork's `hackernews.go`/`hackernews_test.go` are read
end-to-end during review (task 1 below) rather than trusted blindly — this is still a review
pass, not a rubber stamp — but the intent is to land the same design, fixing only what
review finds wrong or stale against current `main`.

**Review checklist** (what "review, not blind copy" concretely means here):
- Does `Fetch`'s signature/error handling still match current `Source`/`CompanyEntry`
  shapes? (`internal/ingest/sources/source.go`)
- Does `routedHTTP` (the test fixture helper, defined in `smartrecruiters_test.go`, shared
  package-wide) still have the same shape the fork's test uses?
- Does `registry.go`'s `All()` still have the same structure at the insertion point the
  fork's diff touched, or has it been refactored since `09076f3e`?
- Do the reused helpers (`sanitizeHTML`, `isRemote`, `NotFuture`, `parseRFC3339`,
  `textContent`/`walk`/`attr`) still have the same signatures?
- Read every line for correctness on its own merits, not just "does it compile" — this is
  the same bar any other adapter PR gets.

## Risks / Trade-offs

- **A post whose header doesn't follow the pipe convention is dropped entirely** (no
  fallback to free-text/LLM extraction) — accepted, matches the fork's own explicit design
  and its measured contributor numbers (244/417 comments ingested); the alternative was the
  discarded LLM pipeline.
- **Board-catalog promotion depends on a human running `harvest_boards.py --hn --write`
  periodically** — same operational-task shape as `harvest-githublists-boards`'s deferred
  group 3, not a gap introduced by this change.

## Migration Plan

1. Extract `internal/ingest/sources/hackernews.go` + `hackernews_test.go` from
   `Manan-Santoki/freehire@1daaa55a`, review against current `main`, fix anything stale.
2. Register in `registry.go`.
3. Add the "Hacker News traps" section to `internal/ingest/sources/AGENTS.md`.
4. `make gen-contracts`.
5. No backfill, no migration — a new source with no prior data.
