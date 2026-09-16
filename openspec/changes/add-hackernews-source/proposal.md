## Why

Issue #2869 proposed `hackernews` as one of three aggregator sources. Earlier drafts of this
change designed a whole new two-stage crawl→LLM-extraction pipeline (modeled on
`internal/ingest/telegram`), on the assumption that HN "who is hiring" comments are free
prose needing an LLM to parse. That assumption was wrong: the thread's own posting
convention is a pipe-delimited header ("Company | Role | Location | ..."), and the
contributor's own fork (`Manan-Santoki/freehire@us-aggregator-sources`,
`internal/ingest/sources/hackernews.go`) already parses it deterministically, with no LLM, as
an ordinary `internal/ingest/sources.Source` adapter — fitting the existing crawl pipeline
directly, with well-covered edge cases (sibling-thread filtering, role-first-segment
detection, commitment/salary/URL-aware location extraction). That code is adopted here
(reviewed, not re-derived from scratch).

Separately, comments that DO name a company via a recognizable ATS link are better served by
a permanent board contribution than a one-off job record — but this needs **no new code**:
`scripts/harvest_boards.py`'s existing `--hn` flag (`harvest_hn()`/`extract_hn_candidates()`)
already crawls the same threads for ATS links and, since PR #2879 fixed
`emit_survivors()`/`existing_slugs()` to dedup against the live `boards` table and emit
`cmd/harvest-boards`-ready seed JSON, already produces exactly the board-contribution
candidates this issue asked for. That half is a periodic operational task (like
harvest-githublists-boards' deferred group 3), not new code.

## What Changes

- Adopt `internal/ingest/sources/hackernews.go` + its test file from the contributor's fork,
  after review (current `main` has moved since the fork's base — recheck against it, not a
  blind copy). Register `NewHackerNews(c)` in `registry.go`'s `All()`.
- Adopt the "Hacker News traps" section the fork's `AGENTS.md` diff already wrote for
  `internal/ingest/sources/AGENTS.md` (not the hiring.cafe/githublists sections from the same
  diff — those are already resolved separately).
- Regenerate `web/src/lib/generated/contracts.ts`'s `SOURCE_VALUES` via `make gen-contracts`.
- No new package, no new migration, no new worker binary, no LLM integration.

## Capabilities

### New Capabilities
- `hackernews-source`: a boardless, aggregator, `fullCatalog` source reading the two newest
  "Ask HN: Who is hiring?" threads via the Algolia HN API, yielding one job per top-level
  comment whose header follows the "Company | Role | ..." convention.

### Modified Capabilities
<!-- none -->

## Impact

- `internal/ingest/sources/hackernews.go`, `hackernews_test.go` (new files, adopted).
- `internal/ingest/sources/registry.go`: register the adapter.
- `internal/ingest/sources/AGENTS.md`: add the "Hacker News traps" section.
- `web/src/lib/generated/contracts.ts`: regenerated `SOURCE_VALUES`.
- No change to `cmd/harvest-boards`, `scripts/harvest_boards.py`, `scripts/ats_boards.py` —
  the board-contribution half already works after PR #2879; running it against the current
  threads is a follow-up operational step, not part of this diff.
- No change to `internal/ingest/telegram` or `cmd/tg-extract`.
