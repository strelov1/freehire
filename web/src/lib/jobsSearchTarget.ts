import { track } from './analytics';
import { displayQuery, type FilterStore } from './filters';
import type { ListSearchTarget } from './listSearch.svelte';
import type { FacetCounts } from './types';

// JobsView registers this with the header search so it drives the list. This holds for the
// standalone /jobs list AND the company page's embedded, scoped list — on
// /companies/:slug the header search filters that company's postings (there's
// no inline box), so both modes route their text search through the header.
// The adapter also exposes `filterScope` (the store + a reactive counts getter)
// so the header renders its Location & work-format popover on these jobs-backed
// lists; the company list registers a bare store with no filterScope, so the
// popover stays hidden there.
//
// `view` is what the list itself owns: its live facet distribution, whether the current
// geography was guessed rather than chosen, and opening its filter modal.
export function jobsSearchTarget(
  filters: FilterStore,
  view: { counts: () => FacetCounts | null; inferred: () => boolean; openFilters: () => void },
): ListSearchTarget {
  return {
    get value() {
      return filters.value;
    },
    commitQuery: (q) => filters.commitQuery(q),
    filterScope: { store: filters, counts: view.counts, variant: 'jobs', inferred: view.inferred },
    suggest: {
      counts: view.counts,
      apply: (suggestion) => {
        // Its own event, not a flag on `search`: the question this answers is how
        // often the dropdown is what puts a facet on. The kind rides along so the
        // empty box's starters can be told apart from a typed completion.
        track('role_suggestion', { role: suggestion.slug, kind: suggestion.kind });
        // A starter comes from the EMPTY box, so there is no typed text to drop —
        // this is an ordinary facet write, the same one the filter modal makes.
        filters.setSign('category', suggestion.slug, 'include');
      },
      applyParts: (plan) => {
        track('role_suggestion', {
          role: plan.facets.map(([, v]) => v).join('+') || (plan.q && displayQuery(plan.q)),
          kind: 'completion',
        });
        filters.applyParts(plan.facets, plan.q ?? '', plan.qFields ?? null);
      },
    },
    openFilters: view.openFilters,
    activeFilters: () => filters.active,
  };
}
