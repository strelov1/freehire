import { api } from '#lib/api.js';
import { latestOnly } from '#lib/latestOnly.js';
import type { FacetCounts } from '#lib/types.js';

/**
 * The jobs feed's live facet distribution (value → count per facet), feeding the dynamic
 * selects (skills, countries) so the user sees which values exist and how many jobs each
 * has under the current filters. A failed fetch leaves the prior counts — the selects
 * degrade to plain (countless) options, never break. latestOnly stops a slow earlier
 * response overwriting a newer one.
 */
export class FeedCounts {
  catalog = $state.raw<FacetCounts | null>(null);
  error = $state.raw<unknown>(null);

  /** `params` is the feed's search: the applied filters plus its fixed scope. */
  constructor(private readonly params: () => URLSearchParams) {}

  refresh = latestOnly(
    () => api.facetCounts(this.params()),
    (facets) => {
      this.catalog = facets;
      this.error = null;
    },
    (error) => (this.error = error),
  );

  /**
   * Live disjunctive facet counts for the staged filter set (built by the modal), merged
   * with the fixed scope params (e.g. company_slug) so every control's counts — and the
   * "Show N jobs" total — match the list. Disjunctive so a selected facet still shows its
   * siblings' counts.
   */
  staged(params: URLSearchParams, scope: Record<string, string>) {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient: built, mutated, passed to the API once; not reactive state
    const p = new URLSearchParams(params);
    for (const [k, v] of Object.entries(scope)) p.set(k, v);
    return api.facetCounts(p, { disjunctive: true });
  }
}
