## Why

The post-run unseen sweep closes a provider's stale jobs, but scopes itself to **the company
slugs the run wrote**. That scope is a proxy for "what did this run cover", and it leaks in the
one place it matters: a company whose last posting drops off a board never enters the crawled
set, so nothing closes its row — ever. `job-lifecycle`'s own requirement states the trade-off
and accepts it, because the alternative considered at the time was closing a whole provider a
partial run had not reached.

The result on prod, measured 2026-09-02 over open non-aggregator postings unseen for 14+ days:

| | rows |
|---|---|
| stale open rows | 321,414 |
| **their board was crawled successfully AFTER the posting was last seen** | **235,313** |
| board has not run since (shard has not come round) — correctly left alone | 72,910 |
| no `board_health` row at all | 7,385 |

235,313 rows sit on boards that ran, listed their content, and did not list these postings.
Nothing closes them. `company_slug=pipe` is the worked example: a 2013 posting from a
`trakstar` board, last seen a month ago, still open and still served (freehire#2328).

## What Changes

- **The sweep gains a second, narrower scope unit: the board.** For each board the run proved
  it covered, the sweep closes that board's open jobs past the grace window — regardless of
  whether the run wrote anything for their company. This is the leak, closed at its source.
- **A board must PROVE it was covered.** It qualifies only when its crawl did not fail AND it
  yielded at least one posting (counting postings the catalogue filter rejected and the
  coverage gate skipped — the crawl reached them either way, which is the same test
  `pipeline.go` already uses to decide a board's crawl was a success).
- **Providers whose crawl reaches only a SLICE of a board are excluded**, keyed on the existing
  `sweepGrace` marker rather than on a list of names. That marker means exactly "the crawl
  deliberately reaches only part of the catalogue", which is precisely when a board-scoped
  close would be wrong. `fullCatalog` and self-closing providers are excluded as they are today.
- **The existing company-scoped close is untouched and still runs.** It covers what the board
  scope cannot: boardless entries, and boards that yielded nothing. The two overlap; the
  overlap is one extra indexed statement per board and it keeps this change additive.
- **Each board-scoped close is logged with its board and count**, so the first runs after
  deploy are readable per board rather than as one provider-level number.

## Impact

- Affected specs: `job-lifecycle`
- Affected code: `internal/ingest/pipeline` (report which boards qualified), `cmd/ingest` (the
  sweep), `internal/platform/db` (one new query).
- **Behavioural:** roughly 215,000 rows close over one fleet cycle — the qualifying subset of
  the 235,313 above, restricted to boards that also yielded on their last run. Catalogue counts
  and `meta.total` fall accordingly. Closing is soft and a re-ingest of a board reopens
  everything it re-lists (`ON CONFLICT` clears `closed_at`), which is the existing remediation
  playbook for a bad sweep.
- **No migration.** The query rides the existing `(source, external_id text_pattern_ops)` index
  through `externalid.BoardPattern`, the same pattern the seen-set already uses.
- **Not in scope:** the ~1,849 rows whose board has left `sources/` entirely, and the 7,385
  with no `board_health` row. Both are tails this rule cannot reach by construction — a board
  that never runs again never qualifies — and both are small enough to leave to the liveness
  probe or a later pass. Recorded here so their absence is a decision rather than an oversight.
