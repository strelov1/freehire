## Why

The post-run unseen sweep closes a provider's stale jobs, but scopes itself to **the company
slugs the run wrote**. A company whose last posting drops off a board never re-enters that set,
so nothing ever closes its row (freehire#2328). Measured on prod: 91% of stale open non-aggregator
rows sit on boards the fleet still crawls successfully — the board listed its content and simply
did not list that company's posting again.

A first attempt at the board-scoped fix (`openspec/changes/close-unseen-by-board/`, shipped as
#2337) closed this leak directly, but closed 110 live `solidjobs` postings within 18 minutes and
was reverted (#2350). The adapter reaches only the first 500 of a board's ~1,400 postings and
reports that as an unqualified success — every qualification condition in that design was
satisfied honestly, and the board still had not been listed in full. That design's own postmortem
concludes an adapter must **opt in** to being sweepable, proven structurally, not inferred from
run behaviour. This change is that opt-in retry.

## What Changes

- **A new per-adapter marker, `fullBoardListing`**, in `internal/ingest/sources`, alongside the
  existing `selfClosing`/`fullCatalog`/`sweepGrace` markers. An adapter earns it only when its
  `Fetch` structurally proves it lists a board to completion — verifies a fetched-count-vs-API
  total match, or paginates to the API's own natural termination with no artificial page/offset
  cap — and returns a hard error (not a partial success) whenever that proof does not hold.
- **The board-scoped close from the reverted change is reinstated**, with its existing
  qualification conditions (crawl did not fail — including a mid-crawl `Failed>0` stream death,
  not just `recordSuccess`'s tolerant verdict; the board yielded at least one posting; the entry
  names a board; the provider is not `sweepGrace`/self-closing/`fullCatalog`) plus one new,
  additive condition: **the provider must carry the `fullBoardListing` marker.** Without it, a
  board never qualifies, regardless of how its crawl went — this is what closes the solidjobs
  hole structurally rather than by exclusion list.
- **A first wave of adapters earn the marker**: the large, high-volume ATS platforms audited
  against the structural bar (exact list finalized during implementation, prioritized by the
  stale-row volume already measured for #2328 — `ukg`, `workday`, `paylocity`, `careerplug` and
  similar). An adapter that cannot cheaply be made to prove completeness is left unmarked and
  deferred — that is a safe default, not a blocker to the rest of this change.
- **The existing company-scoped close is untouched.** It still runs for boardless entries, zero-
  yield boards, and any provider without the new marker.

## Capabilities

### New Capabilities
(none — this extends the existing sweep behaviour)

### Modified Capabilities
- `job-lifecycle`: the unseen sweep gains the board scope described above, gated on the new
  `fullBoardListing` adapter marker.

## Impact

- Affected code: `internal/ingest/sources` (new marker + registry helper + Phase-1 adapter audit
  and any local `Fetch` hardening needed to earn the marker), `internal/ingest/pipeline` (report
  which boards qualified), `cmd/ingest` (the sweep gains the fourth gate), `internal/platform/db`
  (one new query, `CloseUnseenJobsForBoard`).
- **Behavioural:** open jobs on a qualifying board close once their board's next crawl yields
  without re-listing them. Scope is bounded by which adapters earn the marker in Phase 1, so the
  first cycle's close volume is a subset of the 91% figure above, not all of it.
- **No migration.** The query rides the existing `(source, external_id text_pattern_ops)` index.
- **Not in scope:** closing postings when a board's catalog row moves to `status='retired'` —
  a separate, smaller mechanism (~0.6% of the measured problem) with a different trigger
  (curator action on the `boards` table, not crawl behaviour) and no adapter-audit dependency.
  Tracked as its own follow-up rather than folded into this change.
- **Prior art kept, not reused as-is:** `openspec/changes/close-unseen-by-board/` documents the
  reverted attempt and the postmortem that shaped this design; its own code and tasks no longer
  reflect current `main` (that PR was reverted) and are not re-applied verbatim — the query shape,
  `boardReachedPostings` predicate, and per-provider qualifying-boards plumbing are reused as
  reference, re-implemented fresh against current `main`, with the marker gate added.
