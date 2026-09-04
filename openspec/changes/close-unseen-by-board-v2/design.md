## Context

See `proposal.md` for motivation and `openspec/changes/close-unseen-by-board/design.md` for the
prior attempt's full reasoning and its "What production taught this design" postmortem — that
document's constraint is the starting point here: *an adapter must opt in to being sweepable,
proven structurally, because the three run-level conditions it used (crawl didn't fail, board
yielded, board named) test what the RUN did and none of them test what the ADAPTER can do.*

Two things changed since that document was written, both relevant to this design:

- `sources/*.yml` no longer exists; the board catalog is the `boards` Postgres table
  (`internal/ingest/boardcatalog`). This does not affect adapter behaviour or the marker
  mechanism below — adapters are still Go types in `internal/ingest/sources`, and the existing
  `selfClosing`/`fullCatalog`/`sweepGrace` markers already live there unaffected by the retirement.
- The reverted commit (`c7b40ad5f`, revert `9f92f6301`) is fully out of `main`. Its query,
  predicate, and plumbing are sound reference material — the prior design.md's rationale for each
  still holds — but nothing is re-applied verbatim; this change re-implements them against
  current `main` with the marker gate added from the start.

## Goals / Non-Goals

**Goals:**
- Close the 91%-of-rows leak (board still crawled, one company's postings stop appearing) without
  reproducing the solidjobs false-close.
- Make the safety structural: an adapter that cannot prove full-board listing must be unable to
  contribute to the board scope, not merely unlikely to.
- Land a first, bounded wave of adapters carrying the marker, chosen for measured impact.

**Non-Goals:**
- Auditing all ~191 registered adapters. Phase 1 covers the large ATS platforms; a long-tail
  adapter stays on the company scope until a later pass earns it the marker.
- Closing postings when a board's `boards` row moves to `status='retired'`. Separate mechanism,
  separate follow-up issue — see `proposal.md` Impact.
- Rewriting or "fixing" `solidjobs`' pagination cap. It remains unmarked and swept only by the
  company scope; a real pagination fix for it is out of scope here.

## Decisions

### The `fullBoardListing` marker is a Go interface, not data

Alongside `selfClosing`/`fullCatalog`/`sweepGrace` in `internal/ingest/sources/source.go`:
```go
type fullBoardListing interface{ fullBoardListing() }
```
An adapter type implements it (typically a zero-cost marker method) to opt in. `registry.go`
gains `FullBoardListingProviders() map[string]bool` alongside the existing
`SelfClosingProviders`/`FullCatalogProviders`/`SweepGraceWindows` helpers, built the same way —
a type assertion walk over `All()`.

**Alternative considered:** a data-driven flag (e.g. a column on `boards`, or a config map keyed
by provider name). Rejected: completeness is a property of the adapter's *code*, proven by what
`Fetch` does, not an operational fact a curator or config file could assert independently of the
code actually behaving that way. Every existing marker in this file is Go-typed for the same
reason, and mixing shapes for one new marker would be its own inconsistency.

### The bar for earning the marker is structural proof, not documentation

An adapter earns `fullBoardListing` only when its `Fetch`, for every board it crawls:
- verifies a fetched-count-vs-API-reported-total match and returns an error on mismatch, **or**
- paginates to the API's own natural termination (empty page / `hasNext=false` / declared
  `TotalPages` reached) with no artificial page or offset cap in adapter code,

and treats any HTTP or parse failure encountered mid-listing as a hard `Fetch` failure rather than
returning whatever was collected as a partial success.

`internal/ingest/sources/habrcareer.go` already does this for the (unrelated) `fullCatalog`
marker: it fails the whole crawl on any page error specifically because "a partial crawl returned
as success would look like a shrunken catalogue." That is the pattern to replicate, not invent.

**Alternative considered:** self-declaration by code review (add the marker interface to an
adapter after a reviewer reads it and believes it's complete, no code change required). This is
exactly `solidjobs`' failure mode — its limit was documented in a comment, read by nobody's code
and, it turned out, effectively nobody at review time either. Requiring the adapter's own control
flow to enforce the property means a future change to that adapter that breaks completeness has
to also break a test or start erroring loudly, rather than silently drifting back into the
solidjobs shape.

### Sweep integration reuses the prior design's plumbing shape

- **Query**: `CloseUnseenJobsForBoard` — `CloseUnseenJobs` with `company_slug = ANY(...)` swapped
  for `external_id LIKE @board_pattern` (`externalid.BoardPattern`), copying the
  `search_delete_outbox` CTE verbatim so a board-scoped close stays atomic and exact with its
  search-index removal enqueue. No migration; rides the existing
  `(source, external_id text_pattern_ops)` index.
- **Board qualification**: computed in `internal/ingest/pipeline` per board on the run's `Stats`
  (not by widening the `BoardHealth` port — board health answers a board's *health*, the sweep
  asks about *scope*, and routing scope through health would force `cmd/ingest` to re-derive
  through a database round-trip a fact the run already held in memory). Reuses
  `boardReachedPostings` (`Ingested + Rejected + ATSCovered > 0`, deliberately excluding
  `Skipped`, which means "listed but failed to persist" and would let a board prove itself on the
  strength of its own persistence failures).
- **The `Failed>0` refinement is load-bearing, not optional.** The prior attempt's own review
  (tasks 7.1 in the old change) found that `recordSuccess`'s "healthy" verdict tolerates a
  mid-crawl error after partial progress (deliberately, so a rate-limited stream doesn't cool a
  working board) — so a stream that died at posting 40 of 5,000 would otherwise still qualify its
  board, and the sweep would close everything past the point it died: the freehire#725 class of
  bug the whole mechanism exists to avoid. This change's qualification logic must independently
  check `Failed == 0`, separate from — and in addition to — `recordSuccess`'s health verdict.
- **The fourth gate**: `provider ∈ FullBoardListingProviders()`, checked alongside the existing
  `sweepGrace`/self-closing/`fullCatalog` exclusions in the `cmd/ingest` sweep loop.
- `closed_reason` stays `'unseen'` — same mechanism, wider reach, not a new one.

### Phase 1 adapter selection is impact-ordered, not alphabetical

Candidates are the large ATS platforms already implicated by #2328's own measurement (`ukg`
90,215 rows, `workday` 83,315, `paylocity` 25,960, `careerplug` 11,614, and similar — exact
figures to re-measure at implementation time since they will have moved). Each candidate is
audited individually against the structural bar above; a candidate needing a small local `Fetch`
hardening (e.g., turning a soft page cap into a hard error, or adding a total-count check) gets
that fix as part of earning the marker. A candidate that cannot cheaply be made to pass is left
unmarked and deferred — never blocks the rest of the wave.

**Alternative considered:** audit all ~191 adapters in this change. Rejected as disproportionate —
most of the long tail is low-volume, and a rushed audit across that many files is exactly the
condition (reviewer fatigue, one comment nobody double-checks) that produced the solidjobs
incident in the first place.

## Risks / Trade-offs

- **[Risk]** A Phase-1 adapter passes the structural bar today but a future change to it
  reintroduces silent truncation → **Mitigation**: the bar requires the completeness check to
  live in the adapter's own control flow (hard error on failure), so regressing it requires either
  breaking that check visibly or deliberately removing it — not a silent drift, unlike a comment.
- **[Risk]** The audit itself misjudges an adapter as structurally complete when it is not (the
  exact failure class that shipped #2337) → **Mitigation**: the bar is binary and mechanical
  (count-match or natural-termination-with-hard-fail), not a judgment call about probability of
  completeness: it is checkable by reading `Fetch`'s error paths, not by trusting a docstring.
- **[Risk]** Reduced initial impact vs. the original design (only Phase-1 adapters qualify, not
  every board-based provider) → **Accepted**: partial-but-safe closes over one fleet cycle per
  newly-marked adapter, growing as later passes mark more adapters, is the intended shape — see
  proposal.md's "Behavioural" note.
- **[Trade-off]** No row-by-row fallback for a corrupted row inside `CloseUnseenJobsForBoard`
  (mirroring the prior design's choice): at board scope, one bad row's blast radius is already one
  board, unlike the provider-wide statement the 2026-08-11 incident exposed — a second fallback
  code path isn't worth it at this narrower scope.

## Migration Plan

No schema migration. Deploy is a normal code rollout: the new query, marker, and sweep gate ship
together; Phase-1 adapters carry the marker from the same deploy (no adapter earns it via a
follow-up flag flip). First fleet cycle after deploy is the verification point — read the
per-board close log lines (board + count), the same rollout signal the reverted design specified.
Rollback is the existing playbook: closing is soft, and `UpsertJob`'s `ON CONFLICT` reopens
anything a re-ingested board relists, so a bad board-scoped close is recovered by re-crawling that
board — no different from reverting the code change itself if the marker gate turns out wrong for
some adapter.

## Open Questions

- Exact final Phase-1 adapter list and each one's required hardening, if any — resolved per
  adapter during implementation against a fresh volume measurement, not fixed here.
