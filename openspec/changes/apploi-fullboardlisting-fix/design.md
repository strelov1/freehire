## Context

See proposal.md - Why. `apploi.Fetch` already had a correct natural-end signal (a short page)
before this change — the only defect was in how failures around that signal were handled, not
in the signal itself. This makes `apploi` the simplest of the eight adapters audited so far:
no dedup-vs-raw-count issue (no cross-page dedup exists at all), no `unreadableDetail` need (no
per-posting detail fetch — descriptions are inline), and only one proof mechanism to reason
about (unlike `neogov`/`edjoin`, which also accept a source-declared total).

Measuring real board sizes before deciding whether the 10,000-postings-per-employer cap
(`apploiMaxPages × apploiPageSize`) was actually safe to leave untouched hit a genuine
operational obstacle: `jobs` carries no per-board (employer id) column for `apploi` — only
`company_slug`, a coarser grouping that can span several employer/board IDs for one company —
and an unindexed `GROUP BY company_slug` over the ~1.47M-row `apploi` slice of an ~11M-row
table timed out twice against production (8+ and 10+ minutes, still running) before being
cancelled both times via `pg_cancel_backend`. A `TABLESAMPLE SYSTEM (10)` pass completed in
under three minutes and is the measurement this change relies on.

## Goals / Non-Goals

**Goals:**
- Bring `apploi` to the same `fullBoardListing` bar as the eight adapters already fixed: fail
  `Fetch` loudly instead of silently truncating when completeness cannot be structurally
  proven.
- Verify, before deciding not to touch the cap, that no real board is close to it — cheaply
  enough not to burden production with a repeat of the two cancelled full scans.

**Non-Goals:**
- Raising `apploiMaxPages`. The company-level sample found nothing close to the cap; see
  Decisions for why a company-level upper bound is sufficient here even without a per-board
  measurement.
- Adding a `board`/employer-id column to `jobs` so future audits can measure this exactly.
  Worth doing eventually (every adapter that hits this same measurement obstacle would benefit),
  but out of scope for a single-adapter completeness fix.
- Touching the archived/unpublished/private filtering `Fetch` already does — unrelated to
  completeness proof.

## Decisions

**Treat the company-level sampled estimate (~8,700) as sufficient evidence the per-board cap
(10,000) is not being reached, without a per-board measurement.** `company_slug` is a courser
grouping than the API's own `employer` (board) id — one company can operate several employer
accounts, each crawled as its own board — so the largest sampled company's aggregate is an
UPPER BOUND on any single board within it, never a per-board figure itself. An upper bound of
~8,700, comfortably below the 10,000 cap, is enough to conclude no real board is being
truncated today; if the true largest board turned out unexpectedly close to the cap despite
this, the mechanism this change ships (hard-fail on cap exhaustion, per the shared discipline)
converts that into a visible `board_health` failure rather than a silent truncation — the same
safety net every other adapter in this audit relies on for the same class of risk.

**Cancel a slow production query rather than let it keep running.** Both the initial
`GROUP BY company_slug` full-scan attempts were killed via `pg_cancel_backend` once confirmed
still active in `pg_stat_activity` after several minutes, rather than left to finish in the
background unobserved. Two unindexed full scans over a multi-million-row production table are a
real load concern independent of whether the query would eventually return something useful;
the sampled query answers the same question at a small fraction of the cost.

## Risks / Trade-offs

- **[Trade-off]** Same as every other adapter in this audit: a previously-tolerated transient
  later-page hiccup now fails the whole board's crawl for that run. → Accepted, same mitigation
  (`board_health` cooldown/backoff).
- **[Risk]** The cap-safety conclusion rests on a company-level upper bound, not a direct
  per-board measurement (see Context on why the direct measurement was not obtained). →
  Mitigation: this is the same shape of risk `bayt`'s design.md already accepted for its own
  cap decision — if wrong, it fails loudly (a `board_health` entry), not silently.
