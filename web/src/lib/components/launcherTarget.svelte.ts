import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { api } from '#lib/api.js';
import type { ApplyPlan } from '#lib/apiSuggestions.js';
import { browseQuery, planForSuggestion } from '#lib/browseTarget.js';
import type { ListSearchTarget } from '#lib/listSearch.svelte.js';
import type { FacetCounts } from '#lib/types.js';

/**
 * What the header search box searches on a page with no list of its own: a target that
 * navigates to the feed instead of filtering in place, so the box never has to ask which
 * kind of page it is on.
 */
export class LauncherTarget {
  // The distribution behind the empty box on a page with no list of its own. Fetched
  // once, lazily, the first time somebody focuses the box — a page that nobody searches
  // from should not pay for it, and a page that has a list already has the numbers.
  #counts = $state.raw<FacetCounts | null>(null);
  #asked = false;

  /** `pageCounts` is the distribution the rendering page measured, if any; `close`
   *  closes the box's panel before navigating. */
  constructor(
    private readonly pageCounts: () => FacetCounts | null | undefined,
    private readonly close: () => void,
  ) {}

  loadCounts() {
    // Already measured by the page that rendered us — asking again would fetch the
    // identical unfiltered distribution a second time.
    if (this.pageCounts()) return;
    if (this.#asked) return;
    this.#asked = true;
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient: an empty query, passed to the API once
    const params = new URLSearchParams();
    void api
      .facetCounts(params, { facets: ['category'] })
      .then((c) => (this.#counts = c))
      // A missing distribution is not an error here: the empty box simply offers
      // nothing, which is what it did on these pages before.
      .catch(() => {});
  }

  /** Open the feed with a filter. What a pick does off a list page — the ONE thing the
   *  launcher ever did differently, and the reason there were two of these components. */
  browse = (plan: ApplyPlan) => {
    const query = browseQuery(plan);
    // A plan that names nothing navigates nowhere: landing on an unfiltered feed is
    // not what "search for nothing" should do.
    if (query === '') return;
    this.close();
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- query string appended to a resolved path
    void goto(`${resolve('/jobs')}?${query}`);
  };

  readonly target: ListSearchTarget = {
    value: { q: '' },
    commitQuery: (q) => this.browse({ facets: [], q }),
    suggest: {
      counts: () => this.pageCounts() ?? this.#counts,
      apply: (s) => this.browse(planForSuggestion(s)),
      applyParts: this.browse,
    },
  };
}
