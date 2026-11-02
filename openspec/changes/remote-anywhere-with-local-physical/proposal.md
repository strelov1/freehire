## Why

"I'm happy to work remote from anywhere, but hybrid/on-site only in my own city" is
the most common work-format preference there is — and the search language cannot
express it. Facets are ANDed across groups and ORed within one, so today a searcher
must pick between two wrong answers:

- `work_mode=remote` — loses every hybrid/on-site posting at home;
- `work_mode=remote,hybrid,onsite` + a location facet — narrows the remote postings
  to the home geography too, surfacing on-site noise abroad while hiding remote
  work elsewhere.

The profile block already collects the ingredients (`work_modes`, `remote`
reach, `base`) — but "Apply my profile" flattens them into exactly the broken AND
shape, so the stored preference degrades on the way to the results page. Reported
by a user, 2026-11-02: "if I choose remote, I miss non-remote jobs in my city;
if I choose all 3, I see on-site jobs from other countries".

## What Changes

- A new search modifier parameter `work_mode_remote_anywhere=true`, honoured by
  `filterFromValues` — the single pure builder the HTTP handler, the saved-search
  digest matcher and `Narrows` already share — so all three surfaces learn it at
  once.
- When set, the work-mode facet's include group becomes a disjunction:
  `(work_mode IN [selected]) OR (work_mode IN [onsite,hybrid] AND <all location
  facets>)`. With no location facets set the modifier is a no-op (plain OR over the
  selected modes — "remote anywhere" is already what that means); with `remote`
  unselected it is a no-op too. `_mode=and` on `work_mode` disables it (ANDing a
  disjunction is contradictory, and a caller asking `and` is past this feature).
  Location excludes stay global: "not the US" means not the US, in any mode.
- The SPA's "Apply my profile" sets the flag when the profile both accepts remote
  AND accepts hybrid/onsite AND states a base or relocation geography — exactly the
  shape the profile form collects. The filter modal's Work format pane gains the
  matching checkbox, so the flattening is inspectable and editable, not magic.
- `search.UnknownParams` accepts the new param (scalarModifiers), and
  `openapi.yaml` documents it.

## Capabilities

### Modified Capabilities
- `job-search`: the filter language gains the `work_mode_remote_anywhere` modifier
  and its disjunction semantics.
- `remote-region-filters`: unaffected directly, but the spec's "the SPA exposes the
  sentinel" requirement's neighbour — the jobs filter sidebar — now carries one more
  work-format control; `filter-modal`: the Work format pane gains the modifier's
  checkbox and the summary gains its chip.

## Impact

- `internal/search/search/query_filter.go` — read the modifier, build the
  disjunction. `Narrows` needs no change (the modifier alone reads nothing — a
  query carrying only it stays non-narrowing, consistent with the other modifiers).
- `internal/search/search/query_params.go` — `scalarModifiers`.
- `web/src/lib/facetModel.ts` — serialize/parse `work_mode_remote_anywhere`;
  `filtersFromProfile` sets it; `savedSearchQuery` keeps it (alerts must mail the
  same set the user sees); `activeFilterCount` counts it when it changes the answer.
- `web/src/lib/components/filters/FilterModal.svelte` — Work format pane checkbox;
  `FilterSummary.svelte` — chip.
- `web/static/openapi.yaml` — both job-search endpoints.
- No migration, no reindex, no new index attribute: the modifier composes existing
  filterable attributes at query time.
