// The reactive job-filter store, mirrored into the URL. The pure model (types,
// URL (de)serialization, and per-value sign transitions) lives in ./facetModel and
// is re-exported here so existing `$lib/filters` imports keep working; this file
// owns only the SvelteKit-bound FilterStore.

import { UrlSyncedState } from './urlSynced.svelte';
import { saveJobFilters } from './filterStorage';
import {
  type FacetState,
  type JobFilters,
  type Sign,
  type SortField,
  emptyFacet,
  emptyFilters,
  filtersFromParams,
  filtersToParams,
  activeFilterCount,
  facetSetSign,
  facetCycle,
  facetPick,
  facetToggleSign,
  facetAdd,
  facetRemove,
} from './facetModel';

export * from './facetModel';

/** Reactive job filters mirrored into the URL. A thin wrapper over the shared
 *  `UrlSyncedState` primitive: it owns the job-specific shape (facets/visa/salary/
 *  sort) and the discrete-vs-continuous policy, while the primitive owns the
 *  state<->URL transport and the reload debounce. Read `value` to drive inputs and
 *  `applied` (the debounced snapshot) to drive the data reload. */
export class FilterStore {
  #url: UrlSyncedState<JobFilters>;

  /** Seed from the current URL params (passed by the view from `page.url`), so
   *  the same filters render on the server and hydrate on the client. With
   *  `persist`, every explicit change is mirrored to localStorage (an empty set
   *  clears the key), so the standalone /jobs list can restore it after a
   *  navigation back to a bare URL — see JobsView. Navigation re-seeds don't
   *  trigger it (the primitive's `onWrite` fires only on explicit writes), so a
   *  bare URL never wipes the stored set. */
  constructor(initial?: URLSearchParams, persist = false) {
    this.#url = new UrlSyncedState<JobFilters>(
      initial ?? new URLSearchParams(),
      { parse: filtersFromParams, serialize: filtersToParams },
      undefined,
      persist ? (_value, serialized) => saveJobFilters(serialized) : undefined,
    );
  }

  /** Live filters — bind inputs to this. */
  get value(): JobFilters {
    return this.#url.value;
  }

  /** Debounced filters — drive the list/counts reload off this. */
  get applied(): JobFilters {
    return this.#url.applied;
  }

  get active(): number {
    return activeFilterCount(this.#url.value);
  }

  facet(param: string): FacetState {
    return this.#url.value.facets[param] ?? emptyFacet();
  }

  // Continuous inputs (typed/dragged): debounce the reload via setSoon.
  setQuery(q: string) {
    this.#url.setSoon({ ...this.#url.value, q });
  }

  setSalaryMin(n: number | null) {
    this.#url.setSoon({ ...this.#url.value, salaryMin: n });
  }

  // Dragged like the salary slider (a continuous gesture across snap points), so
  // it debounces the reload via setSoon — the URL still updates immediately.
  setPostedWithinDays(n: number | null) {
    this.#url.setSoon({ ...this.#url.value, postedWithinDays: n });
  }

  // Discrete inputs (clicked/toggled): apply immediately via setNow.
  setVisa(on: boolean) {
    this.#url.setNow({ ...this.#url.value, visa: on });
  }

  setSort(sort: SortField) {
    this.#url.setNow({ ...this.#url.value, sort });
  }

  /** Toggle a facet between match-all (AND) and match-any (OR) of its included values. */
  setMatchAll(param: string, on: boolean) {
    this.#setFacet(param, { ...this.facet(param), matchAll: on });
  }

  /** Force a facet value to a specific sign (off/include/exclude). */
  setSign(param: string, v: string, sign: Sign) {
    this.#setFacet(param, facetSetSign(this.facet(param), v, sign));
  }

  /** Pills interaction: cycle a value off → include → exclude → off. */
  cycle(param: string, v: string) {
    this.#setFacet(param, facetCycle(this.facet(param), v));
  }

  /** Select-dropdown interaction: add a value to include, or clear it if selected. */
  pick(param: string, v: string) {
    this.#setFacet(param, facetPick(this.facet(param), v));
  }

  /** Per-chip toggle: flip a value between include and exclude. */
  toggleSign(param: string, v: string) {
    this.#setFacet(param, facetToggleSign(this.facet(param), v));
  }

  /** Add a token to a facet's include set (token inputs); no-op on blank or duplicate. */
  add(param: string, raw: string) {
    this.#setFacet(param, facetAdd(this.facet(param), raw));
  }

  /** Remove a value from a facet entirely (both sets). */
  remove(param: string, v: string) {
    this.#setFacet(param, facetRemove(this.facet(param), v));
  }

  /** Reset a single facet (both sets) — the per-section clear. */
  clearFacet(param: string) {
    this.#setFacet(param, emptyFacet());
  }

  clear() {
    this.#url.setNow(emptyFilters());
  }

  /** Replace the entire filter state from a saved query string and mirror it to
   *  the URL — applies a saved search. Unknown params are ignored by
   *  filtersFromParams, so a stale save degrades to a partial filter. */
  apply(query: string) {
    this.#url.setNow(filtersFromParams(new URLSearchParams(query)));
  }

  /** Re-read filters from the current URL (browser back/forward). */
  syncFromUrl() {
    this.#url.syncFromUrl();
  }

  /** Cancel any pending debounced reload — call from the owning view's cleanup. */
  dispose() {
    this.#url.dispose();
  }

  #setFacet(param: string, st: FacetState) {
    this.#url.setNow({ ...this.#url.value, facets: { ...this.#url.value.facets, [param]: st } });
  }
}
