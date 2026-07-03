// The company filter modal's deferred-edit surface — the company counterpart of
// StagedFilters. Holds a staged copy of the company filters, seeded when the modal
// opens, mutated in memory only; nothing touches the URL or the list until commit().
// Delegates every transition to the pure companyFacetModel, so it stays a thin
// reactive wrapper (like StagedFilters over facetModel).

import type { FacetSelection, FacetStore } from './facets';
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
import type { CompanyFilterStore } from './companyFilters';

export class StagedCompanyFilters implements FacetStore {
  #f = $state<CompanyFilters>(emptyCompanyFilters());

  /** Seed staged state from the applied filters via a serialize→parse round-trip,
   *  which both deep-clones and normalizes (so staged is a plain, independent copy). */
  seed(applied: CompanyFilters): void {
    this.#f = companyFiltersFromParams(companyFiltersToParams(applied));
  }

  /** The staged filters — bind modal controls to this. */
  get value(): CompanyFilters {
    return this.#f;
  }

  /** Total staged filter values (drives the rail counts and the badge). */
  get active(): number {
    return activeCompanyFilterCount(this.#f);
  }

  // Company facets are include-only, so `exclude` is always empty.
  facet(param: string): FacetSelection {
    return { include: this.#f.facets[param] ?? [], exclude: [], matchAll: false };
  }

  // Companies never exclude, so pills' `cycle` and the select's `pick` both collapse
  // to the same plain include toggle.
  cycle(param: string, v: string): void {
    this.#f = toggleCompanyFacet(this.#f, param, v);
  }

  pick(param: string, v: string): void {
    this.#f = toggleCompanyFacet(this.#f, param, v);
  }

  add(param: string, raw: string): void {
    this.#f = addCompanyFacet(this.#f, param, raw);
  }

  remove(param: string, v: string): void {
    this.#f = removeCompanyFacet(this.#f, param, v);
  }

  clearFacet(param: string): void {
    this.#f = clearCompanyFacet(this.#f, param);
  }

  // Inert — the companies endpoint has no exclude/AND-OR modes (FacetStore contract).
  toggleSign(): void {}
  setMatchAll(): void {}

  /** Reset every staged filter. */
  clear(): void {
    this.#f = emptyCompanyFilters();
  }

  /** Staged filters as URL params — for the count preview and commit. */
  params(): URLSearchParams {
    return companyFiltersToParams(this.#f);
  }

  /** Apply staged filters to the live (URL-synced) filter state. */
  commit(live: CompanyFilterStore): void {
    live.apply(this.params().toString());
  }
}
