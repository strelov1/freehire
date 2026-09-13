## Context

See proposal.md - Why. Relevant existing shape:

- `jobview.Job.Source` / `jobs.source` already holds the exact ATS-provider
  vocabulary auto-apply's own code uses (`greenhouse`, `lever`, `ashby`,
  `workable`, `recruitee`, plus every non-ATS crawl source) — confirmed live
  against production data.
- `internal/api/atsapply` already names the providers it can drive:
  `fillProviders` (`layout.go`/`client.go`, chromedp-only: `greenhouse`,
  `lever`) and `browserUseProviders` (`browseruse_fill.go`: `ashby`,
  `workable`). `api` (layer 8) sits strictly above `search` (layer 6) in this
  repo's layering, so `internal/search` cannot import `atsapply` — the
  provider set has to be re-declared, documented as a manually-synced mirror,
  the same shape `atsapply.layouts`/`fillProviders` already keep in sync with
  each other via a same-package containment test.
- `source` is already a public, documented, multi-value `StringFacets` entry
  (`query_filter.go`) but is deliberately withheld from every UI surface
  (`web/src/lib/filterSections.test.ts`'s `NOT_OFFERED` set) — crawl
  provenance isn't a candidate-facing concept. This change does not reopen
  that: it adds a new, derived, semantic boolean, never surfaces raw `source`.
- `search.FromJob` (`internal/search/search/document.go`) is the single
  document builder shared by the full `cmd/reindex` rebuild and the
  incremental `cmd/search-drain` push (confirmed: both call it, no duplicate
  builder). `AIInterview` is computed inline there as
  `view.AIInterviewReports > 0` — the closest existing analog to "a bare
  boolean field derived from something already on the row."
- The UI already has a bespoke-checkbox pattern for a boolean that must never
  become a generic facet-rail entry: `visa`/`hideAIInterview` in
  `web/src/lib/facetModel.ts`, rendered by hand in the filter modal, each
  backed by its own dedicated true-or-absent index attribute.

## Goals / Non-Goals

**Goals:**
- Let a candidate filter to postings whose ATS provider auto-apply can
  currently attempt, with zero new Postgres state and zero backfill worker.
- Keep the signal honest about what it does and doesn't promise (provider
  eligibility, not submission success).

**Non-Goals:**
- Does not change which providers `atsapply` can actually fill/submit for, or
  whether the Ashby/Workable browser-use fallback is enforced in production.
- Does not attempt to model per-attempt obstacles (captcha, unrecognized
  layout, missing answers) as part of this facet — those are only knowable at
  attempt time and are out of scope, per the user's explicit choice to ship a
  best-effort, provider-level signal.
- Does not add this filter to the generic `FACETS`/rail array or the public
  filter-docs page, matching the `visa`/`hideAIInterview` precedent.

## Decisions

**Compute at index-build time from `source`, not as a stored Postgres column.**
Every existing derived boolean facet in this codebase (`is_tech`,
`requires_clearance`) needed a stored column because deriving them requires
reprocessing free-text (a description, an LLM/dictionary pass) that is too
expensive to redo on every index build. This facet's whole input is a single
already-indexed enum column that never needs reprocessing, so recomputing it
inline in `search.FromJob` on every build is strictly simpler and correct by
construction — it can never drift from `source`, and it needs no migration,
no backfill worker, and no `IS DISTINCT FROM`-guarded chunked pass. The only
Postgres-adjacent alternative considered — a generated column or a
`backfill-derive`-style pass — was rejected as pure overhead: it would freeze
a fact that's already free to recompute into a second, redundant source of
truth.

**Duplicate the provider set as a documented constant in `internal/search`,
not a shared package.** Alternatives considered:
- *Import `atsapply`'s maps directly* — impossible under this repo's layering
  (`search` is layer 6, `atsapply` is in `api`, layer 8; `search` may not
  import anything above it).
- *Introduce a new low-layer package (e.g. `internal/dict/atsprovider`) that
  both `search` and `atsapply` import* — a real consolidation, but a bigger
  refactor than this change needs: it would touch `atsapply`'s own
  `fillProviders`/`browserUseProviders` and their existing tests for a
  four-string list that changes rarely and is already cross-checked by
  `atsapply`'s own same-package containment test. Noted as a seam for later,
  not built now — see Open Questions.
- *Duplicate the constant with a doc comment naming it as a manually-synced
  mirror, plus a small test asserting the exact expected set* (chosen) — the
  same shape this codebase already accepts for `atsapply.layouts`
  vs. `fillProviders` themselves; a future change to either side is at least
  a visible, intentional edit to a small, obviously-named list, not silent
  drift.

**True-or-absent via Go's own `omitempty`, no special-casing.** `AIInterview
bool \`json:"ai_interview,omitempty"\`` already gets "true-or-absent" for
free from Go's zero-value `omitempty` behavior — no bespoke marshalling is
needed, and the new field follows the identical pattern.

**A bespoke checkbox, not a new generic facet-rail entry.** The rail system
is the wrong mechanism here for the same reason it's wrong for `source`
itself: it renders a multi-select over raw values, and this facet has
exactly one meaningful state (on/off), same shape as `visa`/`hideAIInterview`.
Reusing their exact wiring (a boolean field in `facetModel.ts`, a bespoke
`<input type="checkbox">` in the filter modal) is less code than teaching the
generic system a new boolean-checkbox control variant for a single caller.

## Risks / Trade-offs

- **The facet can read as more promising than it is.** Ashby/Workable are
  included per explicit product decision even though their browser-use
  fallback ships OFF in production today (100% of those postings currently
  park on submit) → mitigated by the "Best-effort — successful submission
  isn't guaranteed" caption on the checkbox; no code mitigation is in scope,
  since changing what's actually enforced is explicitly out of scope for this
  change.
- **Manual-sync drift.** The exported `search.AutoApplyProviders` can drift
  from `atsapply.fillProviders`/`browserUseProviders` if one changes without
  the other. A same-package unit test in `internal/search/search` only
  catches an accidental edit to the literal itself — it cannot see
  atsapply's real maps, since search (layer 6) cannot import atsapply (in
  api, layer 8) — so on its own it does NOT catch atsapply's side drifting
  away unnoticed, a gap code review caught. → mitigated for real by a second
  test, `TestAutoApplyFacetProvidersMatchThisPackagesOwnMaps` in
  `internal/api/atsapply` (which CAN import search, since api sits above
  it), asserting `search.AutoApplyProviders` equals the live union of
  `fillProviders`/`browserUseProviders`. The two tests together cover both
  directions; full elimination of the duplication would still need the
  shared-package consolidation noted above.
- **The reindex step is manual, like every other filterable-attribute
  rollout in this codebase.** Skipping it leaves pre-existing postings
  unmarked (reads as "auto-apply available nowhere" rather than "index not
  yet caught up") → mitigated by stating it as an explicit task, matching how
  `clearance-facet` documents the same trap.

## Migration Plan

1. Ship the code (document builder field, Meilisearch settings addition,
   query-filter plumbing, frontend checkbox) behind the ordinary deploy
   pipeline.
2. Apply the Meilisearch settings patch (new filterable attribute) to the
   live index BEFORE the binary carrying the new `StringFacets`/`/jobs/facets`
   entry serves traffic — the standing rule this codebase already follows for
   every filterable attribute; skipping the order 500s `/jobs/facets` for
   every caller.
3. After the binary is live, run one full `make reindex` to backfill the
   attribute onto already-indexed open postings from eligible providers.
4. No rollback beyond a normal revert is needed: nothing is stored in
   Postgres, so reverting the code and running one more full reindex removes
   the attribute from the live index cleanly.

## Open Questions

- Whether to consolidate `atsapply`'s provider maps and this change's
  provider constant into one shared low-layer package is left for a future
  change, once (if) the set needs to change and the duplication actually
  causes a drift incident — not blocking this change's specs, approach, or
  tasks.
