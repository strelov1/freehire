## Why

The header search box on the jobs feed is free text only, and the `role` facet that
answers most of what people type there is buried in the filter modal. Five days of
production nginx logs (bots filtered) carry **71,174 searches over 8,340 distinct
queries**; matched against the `roletag` catalogue (1,290 labels + its alias table,
1,750 matchable strings) **37.6% of those searches name a role** — 27.8% exactly
(`design engineer` 2,773, `product manager` 1,234, `product designer` 984,
`data analyst` 910, `ai engineer` 869, `software engineer` 661) and a further 9.8%
as a prefix of one. Over the same window the `role` facet appears in **1.1%** of
requests to `/api/v1/jobs/search`, while `posted_within_days` (49.4%), `countries`
(38.7%) and `work_mode` (27.7%) show the facet UI is otherwise used heavily.

So the gap is discoverability, not relevance: people type the role name because they
never learn the facet exists, and get a full-text match over titles *and*
descriptions where a deterministic title-derived tag would have been strictly more
precise.

## What Changes

- The header list-search input gains a **role suggestion dropdown** on jobs-backed
  lists. Typing two or more characters offers up to five matching roles, each with
  its open-vacancy count, ranked by how well the query names the role and, within one
  such tier, by that count. Count alone was measured wrong on the live catalogue — the
  matcher is alias-aware and typo-tolerant, so it hands first place to whichever
  unrelated role owns the largest bucket. See the spec for the tiers.
- Choosing a suggestion **applies the `role` facet and clears the text query**, so
  the search becomes an exact tag filter that the user can then narrow further with
  seniority, geography, or work format.
- The dropdown always offers a final "search «…» as text" row, so the current
  free-text behaviour stays reachable and nothing is taken away.
- `ListSearchTarget` (the header↔list bridge) gains an optional `roleSuggest`
  capability, published by the jobs views and not by the companies list — the same
  opt-in pattern `filterScope` and `openFilters` already use. The header therefore
  needs no page-path branching.
- **Reverses a prior decision, deliberately:** `HeaderListSearch.svelte` currently
  documents "No dropdown — the page's own list is the live result". That comment is
  replaced along with the behaviour it describes.

No backend, API, migration, index-settings, or reindex work: the role catalogue and
its aliases already ship to the browser in `web/src/lib/generated/contracts.ts`
(`ROLE_LABELS`, `ROLE_ALIASES`), role facet counts are already fetched by the jobs
view, and the `role` query param already filters end to end.

## Capabilities

### New Capabilities

- `role-search-suggestions`: matching a typed query against the role catalogue,
  ranking and bounding the offered roles, and the header dropdown's behaviour —
  what it offers, what selecting a suggestion does, how the keyboard drives it, and
  which surfaces it appears on.

### Modified Capabilities

None. `role-facet` keeps its requirements unchanged — this change only consumes the
slugs and labels that spec already guarantees, and `job-search`'s free-text
behaviour is preserved rather than altered.

## Impact

Frontend only, all under `web/`:

- `web/src/lib/roleSuggest.ts` — new pure matcher/ranker module plus its unit tests.
- `web/src/lib/listSearch.svelte.ts` — one optional member added to
  `ListSearchTarget`.
- `web/src/lib/components/HeaderListSearch.svelte` — renders the dropdown and owns
  its keyboard handling.
- `web/src/lib/components/JobsView.svelte` — publishes `roleSuggest` in the target it
  already registers.
- `web/src/lib/facets.ts` — consumed (`optionMatches` and the role alias map), not
  changed.

Risk is contained to the jobs feed's search box. The `role` facet is derived at index
time, so a job whose title the dictionary cannot place carries no role tag and will
not appear under a role filter; the always-present free-text row is the escape hatch,
and roles the logs show people asking for that the dictionary lacks (`interaction
designer`, `product analyst`, `service designer`, `ui designer`) are recorded here as
dictionary candidates but are explicitly out of scope for this change.
