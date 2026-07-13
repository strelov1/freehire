// Bridge that lets the header act as the single text-search on the list pages
// (/jobs, /companies) which already own a URL-synced filter store. The active
// list view registers its store here on mount; the header's list-mode input
// proxies through it — reusing that store's synchronous URL write, debounced
// reload, and back/forward handling instead of re-implementing (and re-breaking)
// that logic in the header.

import type { FacetStore } from './facets';
import type { FacetCounts } from './types';

/** The slice of a page filter store the header drives. Both FilterStore and
 *  CompanyFilterStore satisfy the base contract (`value.q` + `setQuery`). */
export interface ListSearchTarget {
  readonly value: { q: string };
  setQuery(q: string): void;

  /** The geography (+ work-format) facet scope the header's Location & format popover
   *  drives, present on both list surfaces. `variant` selects the popover body: jobs
   *  lists show work format + the full location pane; the company list shows region +
   *  remote-hiring pills. `counts` is a getter so the view's live facet distribution
   *  stays reactive across the bridge (null on the company list, which fetches none). */
  readonly filterScope?: {
    store: FacetStore;
    counts(): FacetCounts | null;
    variant: 'jobs' | 'companies';
  };
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
