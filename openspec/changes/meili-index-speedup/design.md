## Context

The `jobs` facet index (Meilisearch v1.49.0, `internal/search/client.go:facetSettings`)
is written by two paths: `cmd/reindex` (full swap-rebuild, multi-hour schedule) and
`cmd/search-drain` (incremental, drains `search_outbox` in waves of ~200 documents,
see `internal/searchdrain/AGENTS.md`). Both were observed slow in production —
`search_outbox` has backlogged to ~90k pending entries — and a local benchmark
against a real ~317k-document sample of prod data reproduced the cost and, via
Meilisearch's own `progressTrace`, isolated it: the `merging word proximity` phase
(building `wordPairProximityDocids`, a structure Meilisearch reported as *larger than
the documents themselves*) dominates batch time, and word-prefix post-processing adds
a smaller but consistent cost on top.

## Goals / Non-Goals

**Goals:**
- Cut per-batch indexing cost for both `cmd/reindex` and `cmd/search-drain` by
  eliminating the most expensive, skippable indexing phase.
- Keep the change to index *settings* only — no new dependency, no schema/migration,
  no change to `internal/searchdrain`'s or `cmd/reindex`'s own logic.

**Non-Goals:**
- Not attempting to fix the *architectural* cause (Meilisearch re-merges structures
  across the whole live index on most writes) — that is upstream and out of scope.
- Not changing `SEARCH_DRAIN_BATCH_SIZE` or any queue/worker tuning knob; this
  targets the cost of a single push, not the batching strategy around it.
- Not touching `semanticSettings` (the vector/embedding index) — this only concerns
  the plain keyword/facet index the proposal's benchmark exercised.
- **Not disabling `prefixSearch`**, despite it being part of the same benchmark's
  savings — see Decision 2 (reverted after code review).

## Decisions

**1. `ProximityPrecision: "byAttribute"` (skip per-word-pair distance)**
Meilisearch's default (`byWord`) computes and stores the exact word-to-word distance
across the whole document set; `byAttribute` only checks whether query terms share an
attribute. The benchmark's `progressTrace` showed the `merging word proximity` step —
up to ~10s of a ~16s batch — vanish entirely under `byAttribute`, with no
`wordPairProximityDocids` structure built at all. Trade-off: queries where two words
being *close together* (not just co-present) matters for ranking lose that signal.
Job descriptions are long-form prose, not short phrases where adjacency is load-bearing,
and Meilisearch's own guidance is that the relevancy impact is minor in most use
cases — accepted.

**2. `PrefixSearch` left at its default (NOT disabled) — reverted after code review**
The original version of this design proposed also disabling `prefixSearch`, reasoning
that "freehire's job search is a submit-and-search form, not a live-as-you-type
autocomplete surface." Code review challenged that claim and it did not survive
verification: `web/src/lib/components/HeaderSearch.svelte` (the global Cmd+K
launcher) debounces 250ms and calls `api.searchJobs` with the in-progress query text
on every pause, and `web/src/lib/filters.ts`'s `setQuery` does the same for the
`/jobs` list page via a debounced reload. `internal/search/client.go`'s search path
passes the raw query straight into Meilisearch with no `MatchingStrategy` override,
so Meilisearch's default last-word prefix matching is exactly what lets a paused
mid-word query like `"engin"` match `"engineer"` today. Disabling `prefixSearch`
would have broken both of these live-search surfaces mid-word. Rejected; the setting
stays at Meilisearch's default (`indexingTime`).

**3. Settings-only change, no code restructuring**
`ProximityPrecision` is a pure `meilisearch.Settings` field on the existing
`facetSettings()` function's return — added, not swapped in as a separate settings
variant — because it applies globally to `SearchableAttributes` and is not the kind
of setting that would ever legitimately differ between a reindex-built and a
drain-built version of the same index (both write into the same live index).

## Risks / Trade-offs

- **[Risk] Stale index between deploy and reindex** — the code deploys before the
  settings are applied. → **Mitigation**: none needed beyond the existing operational
  discipline — a settings change already requires a `reindex` to take effect (see the
  existing comment on `FilterableAttributes` in `client.go`: "Adding a new filterable
  attribute needs a reindex before it takes effect"). This is the same class of
  change, not a new risk class.
- **[Risk] Search relevancy shift** — some queries may rank slightly differently once
  proximity is attribute-scoped rather than word-scoped. → **Mitigation**: no
  mitigation planned pre-emptively; if a regression is reported post-deploy, revert
  is a one-line settings change plus a reindex.
- **[Risk] The full `reindex` required to apply this is itself the expensive
  operation being optimized** — running it once more to pick up the new settings adds
  one more multi-hour cycle at the OLD (slow) settings, since the settings only take
  effect from the reindex that applies them onward, not retroactively on data already
  on disk mid-run. → **Mitigation**: this is a one-time cost inherent to any settings
  change, not specific to this one; no different from any other`FilterableAttributes`
  change already accepted in this codebase.

## Migration Plan

1. Merge the settings change to `facetSettings()`.
2. Deploy (per repo's normal deploy flow — no schema/migration involved).
3. Run a full `cmd/reindex` once, per `internal/searchdrain/AGENTS.md`'s existing
   `freehire-reindexw` mechanism, which already pauses `freehire-search-drain.timer`
   for the duration and resumes it on exit. No manual runbook step is needed beyond
   the existing "reindex after a settings change" discipline.
4. Verify: after the reindex completes, `search_outbox` backlog should drain
   noticeably faster; spot-check a `search-drain` batch's Meilisearch task duration
   via `GET /tasks/{id}` — the `merging word proximity` progressTrace entry should be
   absent.
5. **Rollback**: revert the `ProximityPrecision` field, redeploy, run `reindex`
   again. No data loss risk — settings-only, not a data migration.

## Open Questions

None — validated experimentally (see proposal) before proposing this change.
