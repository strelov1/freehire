// The catalogue filter modal's deferred-edit surface — the Talent Network counterpart of
// StagedFilters and StagedCompanyFilters. Holds a staged copy of the filters, seeded when
// the modal opens, mutated in memory only; nothing touches the URL or the list until
// commit(). Delegates every transition to the pure talentFacetModel, so it stays a thin
// reactive wrapper.

import type { FacetSelection, FacetStore } from './facets';
import {
  activeTalentFilterCount,
  addTalentFacet,
  clearTalentFacet,
  emptyTalentFilters,
  removeTalentFacet,
  setTalentMinYears,
  talentFiltersFromParams,
  talentFiltersToParams,
  toggleTalentFacet,
  type TalentFilters,
} from './talentFacetModel';
import type { TalentFilterStore } from './talentFilters';

export class StagedTalentFilters implements FacetStore {
  #f = $state<TalentFilters>(emptyTalentFilters());

  /** Seed staged state from the applied filters via a serialize→parse round-trip, which
   *  both deep-clones and normalises (so staged is a plain, independent copy). */
  seed(applied: TalentFilters): void {
    this.#f = talentFiltersFromParams(talentFiltersToParams(applied));
  }

  /** The staged filters — bind modal controls to this. */
  get value(): TalentFilters {
    return this.#f;
  }

  /** Total staged filter values (drives the rail counts and the badge). */
  get active(): number {
    return activeTalentFilterCount(this.#f);
  }

  /** The staged filters as URL parameters — what the modal previews counts against, and
   *  what commit() hands the live store. A method rather than a getter because that is
   *  the shape FilterModalShell's StagedSurface asks for. */
  params(): URLSearchParams {
    return talentFiltersToParams(this.#f);
  }

  // Catalogue facets are include-only, so `exclude` is always empty.
  facet(param: string): FacetSelection {
    return { include: this.#f.facets[param] ?? [], exclude: [], matchAll: false };
  }

  // The catalogue never excludes, so the pills' `cycle` and the select's `pick` both
  // collapse to the same plain include toggle.
  cycle(param: string, v: string): void {
    this.#f = toggleTalentFacet(this.#f, param, v);
  }

  pick(param: string, v: string): void {
    this.#f = toggleTalentFacet(this.#f, param, v);
  }

  add(param: string, raw: string): void {
    this.#f = addTalentFacet(this.#f, param, raw);
  }

  remove(param: string, v: string): void {
    this.#f = removeTalentFacet(this.#f, param, v);
  }

  clearFacet(param: string): void {
    this.#f = clearTalentFacet(this.#f, param);
  }

  // No exclude and no AND/OR mode on this endpoint, so these are inert — present only to
  // satisfy the FacetStore contract.
  toggleSign(): void {}
  setMatchAll(): void {}

  /** Years is a threshold rather than a value set, so it sits outside FacetStore and the
   *  control calls it directly. */
  setMinYears(years: number | undefined): void {
    this.#f = setTalentMinYears(this.#f, years);
  }

  clear(): void {
    this.#f = emptyTalentFilters();
  }

  /** Publish the staged copy to the live store, which mirrors it into the URL. */
  commit(store: TalentFilterStore): void {
    store.apply(this.params().toString());
  }
}
