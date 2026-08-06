## Context

`internal/sources/workday.go:listPostings` pages a Workday board's CXS jobs-listing
API, latching the first non-zero `total` it sees and paging until reaching it. Some
tenants cap the reported `total` at 2,000 and, past `offset=2000`, return page 1 again
rather than an error or an empty page — so on an affected board the crawl exits
cleanly having read at most 2,000 postings, with no signal that it stopped short.
Confirmed live (2026-08-06) against `accenture.wd103.myworkdayjobs.com/AccentureCareers`:
`total: 2000` vs. a `jobFamilyGroup` facet-count sum of ~51,500; `offset=0` and
`offset=2000` return the identical posting. Full write-up and reproduction commands:
`docs/superpowers/specs/2026-08-06-workday-facet-split-design.md` (Superpowers spec
from the brainstorming session that produced this design; this document restates it
for OpenSpec tracking).

A random sample of 120 boards from `sources/workday.yml` (seed 11) found the ceiling
on 2 of 119 reachable boards (~1.7%), extrapolating to roughly 110 of the file's
6,462 boards — skewed toward the largest employers.

Separately, `internal/sources/http.go:493-519` returns immediately on any HTTP 403
(the `default` switch branch), failing the whole board. `eightfold.go` already treats
403 as a retryable rate-limit signal for that provider specifically (Eightfold uses
403 for throttling, not auth).

## Goals / Non-Goals

**Goals:**
- A single Workday crawl retrieves every posting on a board, including ones where the
  board's own `total` is capped at 2,000.
- No extra "probe" requests beyond what listing already returns — the facet counts
  needed to plan subdivision arrive in the response already being parsed.
- No hardcoded per-company logic; the mechanism must work for any of the ~110
  affected boards, not just the ones already found.
- A transient 403 no longer fails the whole board.
- Boards that were never capped see zero behavior change.

**Non-Goals:**
- Not fixing the general "total reported only on page one, `total:0` thereafter" case
  — that is already handled by the existing latch and is a different failure mode.
- Not building a generic cross-provider facet-subdivision abstraction. Nothing today
  suggests another provider needs this; it stays Workday-local.
- Not guaranteeing 100% completeness on every conceivable board shape. Recursion
  depth is bounded; exhausting it without resolving the cap is logged, not silently
  dropped, but the board may still cap out on some branches.

  **Confirmed live, not just theoretical**: Accenture's "Software Engineering" job
  family alone (~24,221 postings) exhausts `maxFacetDepth` on its worst remaining
  branch. Within it, Workday's other facet dimensions are degenerate for this
  tenant — `timeType` is 99.96% "Full time" (max value 24,211 of 24,221, useless as
  a splitter) and `locationMainGroup` returns no values at all once `jobFamilyGroup`
  is applied — leaving `workerSubType` as the only real second dimension, whose own
  best (smallest-max) remaining bucket ("Release Management") is still 5,098, over
  the cap, with no third useful dimension left to apply. This is a genuine ceiling
  of "subdivide by native facets" for this specific tenant's data shape, not a code
  defect: the exhausted branch still returns up to 2,000 *distinct, correct*
  postings via ordinary pagination (never past offset 2,000, so it never triggers
  the wraparound bug this change exists to fix) — strictly better than today's
  single 2,000-item ceiling for the *entire* board, even though it falls short of
  that one branch's true ~24k. Confirmed live (2026-08-06) that NVIDIA (2,617
  postings, fully resolved, no exhaustion) and Accenture's other ~15 job families
  are unaffected by this — it is specific to Accenture's one or two largest
  families.

## Decisions

**Detect a capped board by `total == 2000` on page 1.** A board that genuinely has
exactly 2,000 postings triggers the same path harmlessly — subdivision still returns
the correct set, just via more requests than strictly necessary. No false negatives;
false positives cost requests, not correctness. Alternative considered: probing
`offset=2000` for a repeated posting before deciding. Rejected — an extra round trip
per board for a signal `total == 2000` already gives for free.

**Subdivide via the listing response's own `facets` field, recursing across
*different* dimensions rather than deeper into the same one.** Workday's listing
response carries `facets: [{facetParameter, values: [{id, count}]}]`. Re-querying with
`appliedFacets: {"<param>": ["<id>"]}` scopes both postings and the *other* facet
dimensions in the response (verified live: `jobFamilyGroup=Security` → `total: 817`,
matching that facet value's own count). The filtered dimension does not rescope
against itself — Workday returns the same global breakdown for it — so narrowing an
oversized slice further means adding a facet dimension not yet applied in this
recursion branch (e.g. `workerSubType`, `timeType`, `locationMainGroup`; verified live
that `workerSubType` properly rescopes under an applied `jobFamilyGroup` filter).
At each level, pick the not-yet-used dimension whose *largest single value* is
*smallest* — the dimension that leaves the smallest oversized remainder, without
hardcoding dimension names or order. (The inverse heuristic — pick the dimension
with the single biggest value anywhere — was tried first and, live against
Accenture, kept selecting a dimension whose top bucket was still ~2000-sized,
making no measurable progress across repeated depth-3 exhaustions before this was
corrected during the live dry run in task 7.4.)

**Bound recursion at `maxFacetDepth = 3` combined dimensions.** Verified against the
worst known case: Accenture's top-level split by `jobFamilyGroup` still leaves
"Software Engineering" at ~24k; a second, different dimension resolves it. Three
combined dimensions gives headroom beyond the worst case observed without unbounded
recursion. Alternative considered: matching `ats-scrapers`' `MAX_SUBDIVISION_DEPTH=4`
exactly. Rejected as unmotivated by any observed freehire board — 3 already covers the
worst case found; deeper is easy to raise later if a board needs it, which the depth-
exhaustion log line will surface if it happens.

**Dedup collected postings by `ExternalPath` across all recursed slices.** Some facet
dimensions multi-count (a posting can carry more than one `workerSubType`), so a
posting can surface in more than one slice. `ExternalPath` is the same identity key
`crawl` already keys the pipeline's dedup on (`jobs.UNIQUE (source, external_id)`),
so an over-inclusive split is safe by construction — it costs extra requests, never
extra rows.

**403 retry is a Workday-local wrapper, not a shared-client change.** Mirrors
`eightfold.go`'s `getJSONRetrying`/`isRateLimited` shape. 403 semantics are provider-
specific — Eightfold: rate-limit; some Workday tenants: CSRF/auth per `ats-scrapers`'
own note — so treating it as fatal-by-default in the shared client and opting in per
provider (the existing convention) is kept rather than changing the shared default.

## Risks / Trade-offs

- **[Request volume increase on affected boards]** → Bounded to the ~110 capped
  boards; unaffected boards see zero change. Runs under the crawl's existing
  concurrency and retry handling — no new rate-limiting mechanism introduced. If a
  board's request volume under subdivision proves too aggressive in practice, the
  existing 403-retry backoff (this same change) absorbs it. **Confirmed live**: the
  listing walk is sequential (no new concurrency added), so a board Accenture's size
  takes on the order of tens of minutes wall-clock for its full facet tree, which a
  scratch dry run without the crawler's normal per-board time budget could not fully
  observe end-to-end. Watch actual crawl duration against the shard timeout at
  rollout (tasks.md 8.3) the way `workday-hydrating-crawl` tracked Dollar Tree's.
- **[A board lacks any usable facet dimension, or exhausts depth without resolving]**
  → Logged via `log.Printf` (matching this package's existing convention in
  `config.go`, `eightfold.go`, `djinni.go`) rather than silently truncating, and the
  adapter returns what it collected — strictly better than today's fully silent
  2,000-item ceiling, even where it can't fully resolve.
- **[Facet id/value semantics drift between Workday API versions or tenants]** →
  Mitigated by driving the split entirely off data returned in the response itself
  (facet ids and counts), not hardcoded facet names beyond the generic dimension-
  selection heuristic.

## Migration Plan

No migration. This is a behavior-only fix to one adapter's crawl path; no schema,
config, or API changes. Deploys as a normal code change and takes effect on the next
scheduled Workday ingest run. Rollback is a plain revert — no data backfill needed,
since the existing 48h unseen-sweep and next-crawl drift already tolerate a
temporarily smaller catalogue on a capped board (today's status quo).

## Open Questions

None outstanding — scope and approach were confirmed with the user during
brainstorming (see `docs/superpowers/specs/2026-08-06-workday-facet-split-design.md`):
fix both the 2,000-cap and the 403-fatal gap together, via a generic adaptive
facet-split rather than a hardcoded per-company list.
