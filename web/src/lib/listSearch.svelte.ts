// Bridge that lets the header act as the single text-search on the list pages
// (/jobs, /companies) which already own a URL-synced filter store. The active
// list view registers its store here on mount; the header's list-mode input
// proxies through it — reusing that store's synchronous URL write, debounced
// reload, and back/forward handling instead of re-implementing (and re-breaking)
// that logic in the header.

import type { FacetStore } from './facets';
import type { ApplyPlan } from './apiSuggestions';
import type { Suggestion } from './suggestions';
import type { FacetCounts } from './types';

/** The slice of a page filter store the header drives. Both FilterStore and
 *  CompanyFilterStore satisfy the base contract (`value.q` + `commitQuery`).
 *
 *  The header COMMITS; it does not type into the store. What the visitor types is a
 *  draft the header holds until Enter, a chosen suggestion, or the clear button — so
 *  every write arriving here is a discrete act, and the store's debounced `setQuery`
 *  (which exists for continuous input) would only delay it. */
export interface ListSearchTarget {
  readonly value: { q: string };
  commitQuery(q: string): void;

  /** The geography (+ work-format) facet scope the header's Location & format popover
   *  drives, present on both list surfaces. `variant` selects the popover body: jobs
   *  lists show work format + the full location pane; the company list shows region +
   *  remote-hiring pills. `counts` is a getter so the view's live facet distribution
   *  stays reactive across the bridge (null on the company list, which fetches none). */
  readonly filterScope?: {
    store: FacetStore;
    counts(): FacetCounts | null;
    variant: 'jobs' | 'companies';
    /** Whether the current geography was guessed from the visitor's IP country
     *  rather than chosen. A getter, like `counts`, so the trigger stops saying it
     *  the moment the visitor edits the scope. Absent on surfaces that never guess,
     *  which read as "chosen" — the honest default, since they were. */
    inferred?(): boolean;
  };

  /** Suggestions under the header's search input, present only on jobs-backed lists —
   *  roles and categories are jobs facets, so the companies list publishes nothing
   *  here and the header renders no dropdown there without knowing which page it is
   *  on. Same opt-in shape as `filterScope` above.
   *
   *  Every member is a getter so the distributions stay reactive across the bridge.
   *
   *  `counts` is the ordinary scoped distribution, and it serves the EMPTY box only —
   *  empty means no text query is narrowing it, and the visitor's location and other
   *  filters SHOULD be reflected in what they are offered as a starting point. What a
   *  TYPED box offers comes from the suggestions endpoint, which reads the catalogue's
   *  own vocabulary rather than any distribution shipped to the browser. */
  readonly suggest?: {
    counts(): FacetCounts | null;
    /** Apply a locally-built starter row: one facet, named by its own kind. */
    apply(suggestion: Suggestion): void;
    /** Apply a completion from the suggestions endpoint, which composes SEVERAL
     *  values at once — a role plus a company, or a free-text title. They go in one
     *  write, because applying them one at a time would reload the list per part and
     *  leave it briefly filtered by half a query. */
    applyParts(plan: ApplyPlan): void;
  };

  /** Opens the page's own filter modal, and reports its active-filter count, so the
   *  header can host the All-filters trigger. Present on list pages that own a filter
   *  modal (jobs feed, company page, companies list); absent on the launcher, where the
   *  header renders no trigger. `activeFilters` is a getter so the badge tracks the
   *  view's reactive filter state across the bridge. */
  readonly openFilters?: () => void;
  readonly activeFilters?: () => number;
}

let active = $state<ListSearchTarget | null>(null);

/** The current list page's search target, or null off the list pages. Reactive —
 *  read it in the header to bind the input. */
export function listSearchTarget(): ListSearchTarget | null {
  return active;
}

/** A list view registers its store on mount and clears it (null) on destroy. */
export function setListSearchTarget(target: ListSearchTarget | null): void {
  active = target;
}
