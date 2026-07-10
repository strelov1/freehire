// The modal's deferred-edit surface. It holds a staged copy of the filters, seeded
// from the applied state when the modal opens, and mutated in memory only — nothing
// touches the URL or the job list until commit(). This keeps the live-URL invariant
// (FilterStore is the single applied truth) intact while the modal edits freely.
//
// It satisfies the same FacetStore contract the sidebar controls drive, so the exact
// same facet controls render against staged or live state without change.

import type { FacetStore } from './facets';
import type { UserProfile } from './types';
import {
  activeFilterCount,
  emptyFacet,
  emptyFilters,
  filtersFromParams,
  filtersFromProfile,
  filtersToParams,
  facetCycle,
  facetPick,
  facetToggleSign,
  facetAdd,
  facetRemove,
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

  cycle(param: string, v: string): void {
    this.#setFacet(param, facetCycle(this.facet(param), v));
  }

  pick(param: string, v: string): void {
    this.#setFacet(param, facetPick(this.facet(param), v));
  }

  toggleSign(param: string, v: string): void {
    this.#setFacet(param, facetToggleSign(this.facet(param), v));
  }

  add(param: string, raw: string): void {
    this.#setFacet(param, facetAdd(this.facet(param), raw));
  }

  remove(param: string, v: string): void {
    this.#setFacet(param, facetRemove(this.facet(param), v));
  }

  clearFacet(param: string): void {
    this.#setFacet(param, emptyFacet());
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

  /** Reset every staged filter and seed from a user profile: specializations → `category`,
   *  skills → `skills`, and the location block → the location facets (see filtersFromProfile).
   *  Backs the modal's "Apply my profile" action. */
  applyProfile(profile: UserProfile): void {
    this.#f = filtersFromProfile(profile);
  }

  /** Replace the staged filters from a saved query string — the "My filters" tab
   *  seeds the staged copy so selecting a set previews (and applies on Show results)
   *  rather than committing to the live URL immediately. */
  apply(query: string): void {
    this.#f = filtersFromParams(new URLSearchParams(query));
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
