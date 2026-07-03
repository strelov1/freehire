// The modal's deferred-edit surface. It holds a staged copy of the filters, seeded
// from the applied state when the modal opens, and mutated in memory only — nothing
// touches the URL or the job list until commit(). This keeps the live-URL invariant
// (FilterStore is the single applied truth) intact while the modal edits freely.
//
// It satisfies the same FacetStore contract the sidebar controls drive, so the exact
// same facet controls render against staged or live state without change.

import type { FacetStore } from './facets';
import {
  activeFilterCount,
  emptyFacet,
  emptyFilters,
  filtersFromParams,
  filtersToParams,
  type FacetState,
  type FilterStore,
  type JobFilters,
} from './filters';

export class StagedFilters implements FacetStore {
  #f = $state<JobFilters>(emptyFilters());

  /** Seed staged state from the applied filters via a serialize→parse round-trip,
   *  which both deep-clones and normalizes (so staged is a plain, independent copy). */
  seed(applied: JobFilters): void {
    this.#f = filtersFromParams(filtersToParams(applied));
  }

  /** The staged filters — bind modal controls to this. */
  get value(): JobFilters {
    return this.#f;
  }

  /** Total staged filter values (drives the rail counts and the badge). */
  get active(): number {
    return activeFilterCount(this.#f);
  }

  facet(param: string): FacetState {
    return this.#f.facets[param] ?? emptyFacet();
  }

  toggle(param: string, v: string): void {
    const st = this.facet(param);
    const has = st.values.includes(v);
    this.#setFacet(param, { ...st, values: has ? st.values.filter((x) => x !== v) : [...st.values, v] });
  }

  add(param: string, raw: string): void {
    const v = raw.trim();
    const st = this.facet(param);
    if (!v || st.values.includes(v)) return;
    this.#setFacet(param, { ...st, values: [...st.values, v] });
  }

  remove(param: string, v: string): void {
    const st = this.facet(param);
    this.#setFacet(param, { ...st, values: st.values.filter((x) => x !== v) });
  }

  clearFacet(param: string): void {
    this.#setFacet(param, emptyFacet());
  }

  setExclude(param: string, exclude: boolean): void {
    this.#setFacet(param, { ...this.facet(param), exclude });
  }

  setMatchAll(param: string, on: boolean): void {
    this.#setFacet(param, { ...this.facet(param), matchAll: on });
  }

  setSalaryMin(n: number | null): void {
    this.#f = { ...this.#f, salaryMin: n };
  }

  setVisa(on: boolean): void {
    this.#f = { ...this.#f, visa: on };
  }

  setPostedWithinDays(n: number | null): void {
    this.#f = { ...this.#f, postedWithinDays: n };
  }

  /** Reset every staged filter. */
  clear(): void {
    this.#f = emptyFilters();
  }

  /** Staged filters as URL params — for the count preview and commit. */
  params(): URLSearchParams {
    return filtersToParams(this.#f);
  }

  /** Apply staged filters to the live (URL-synced) filter state. */
  commit(live: FilterStore): void {
    live.apply(this.params().toString());
  }

  #setFacet(param: string, st: FacetState): void {
    this.#f = { ...this.#f, facets: { ...this.#f.facets, [param]: st } };
  }
}
