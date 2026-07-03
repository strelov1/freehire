// Reactive company-catalog filters mirrored into the URL. The pure model (types,
// serialization, mutators) lives in companyFacetModel.ts (unit-testable, $app-free);
// this module owns only the reactive UrlSyncedState wrapper. Re-exports the model's
// public surface so existing `$lib/companyFilters` importers are unchanged.

import { COMPANY_FACETS, type FacetSelection, type FacetStore } from './facets';
import {
  activeCompanyFilterCount,
  addCompanyFacet,
  clearCompanyFacet,
  companyFiltersFromParams,
  companyFiltersToParams,
  emptyCompanyFilters,
  removeCompanyFacet,
  toggleCompanyFacet,
  type CompanyFilters,
} from './companyFacetModel';
import { UrlSyncedState } from './urlSynced.svelte';

export {
  activeCompanyFilterCount,
  companyFiltersFromParams,
  companyFiltersToParams,
  emptyCompanyFilters,
  type CompanyFilters,
};

/** Reactive company filters mirrored into the URL — a thin wrapper over the shared
 *  UrlSyncedState primitive, satisfying the FacetStore contract so the same
 *  FacetSection/controls render it. Read `value` to drive inputs and `applied` (the
 *  debounced snapshot) to drive the list reload. */
export class CompanyFilterStore implements FacetStore {
  #url: UrlSyncedState<CompanyFilters>;

  constructor(initial?: URLSearchParams) {
    this.#url = new UrlSyncedState<CompanyFilters>(initial ?? new URLSearchParams(), {
      parse: companyFiltersFromParams,
      serialize: companyFiltersToParams,
    });
  }

  /** Live filters — bind inputs to this. */
  get value(): CompanyFilters {
    return this.#url.value;
  }

  /** Debounced filters — drive the list reload off this. */
  get applied(): CompanyFilters {
    return this.#url.applied;
  }

  get active(): number {
    return activeCompanyFilterCount(this.#url.value);
  }

  // Company facets are include-only (no `_exclude` on the companies endpoint), so the
  // selection maps its value set to `include` and leaves `exclude` empty.
  facet(param: string): FacetSelection {
    return { include: this.#url.value.facets[param] ?? [], exclude: [], matchAll: false };
  }

  // Free text debounces the reload (setSoon); the URL still updates synchronously.
  setQuery(q: string) {
    this.#url.setSoon({ ...this.#url.value, q });
  }

  // Companies never exclude, so both the pills' `cycle` (off → include → exclude) and
  // the select's `pick` collapse to the same plain include toggle.
  cycle(param: string, v: string) {
    this.#url.setNow(toggleCompanyFacet(this.#url.value, param, v));
  }

  pick(param: string, v: string) {
    this.#url.setNow(toggleCompanyFacet(this.#url.value, param, v));
  }

  add(param: string, raw: string) {
    this.#url.setNow(addCompanyFacet(this.#url.value, param, raw));
  }

  remove(param: string, v: string) {
    this.#url.setNow(removeCompanyFacet(this.#url.value, param, v));
  }

  clearFacet(param: string) {
    this.#url.setNow(clearCompanyFacet(this.#url.value, param));
  }

  // The companies endpoint has no exclude/AND-OR modes and no company facet opts
  // into them, so these are inert — present only to satisfy the FacetStore contract.
  toggleSign() {}
  setMatchAll() {}

  clear() {
    this.#url.setNow(emptyCompanyFilters());
  }

  /** Replace the entire filter state from a query string and mirror it to the URL —
   *  the commit target for the deferred filter modal (mirrors FilterStore.apply). */
  apply(query: string) {
    this.#url.setNow(companyFiltersFromParams(new URLSearchParams(query)));
  }

  syncFromUrl() {
    this.#url.syncFromUrl();
  }

  dispose() {
    this.#url.dispose();
  }
}
