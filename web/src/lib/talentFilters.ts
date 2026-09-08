// Reactive Talent Network catalogue filters mirrored into the URL. The pure model (types,
// serialisation, mutators) lives in talentFacetModel.ts — unit-testable and free of
// `$app`; this module owns only the reactive UrlSyncedState wrapper, the same split
// companyFilters.ts has over companyFacetModel.ts.

import { type FacetSelection, type FacetStore } from './facets';
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
import { UrlSyncedState } from './urlSynced.svelte';

/** Reactive catalogue filters mirrored into the URL — a thin wrapper over the shared
 *  UrlSyncedState primitive, satisfying the FacetStore contract so the same
 *  FacetSection/ChipFacet controls the job and company lists use render this one too.
 *  Read `value` to drive inputs and `applied` (the debounced snapshot) to drive the
 *  list reload. */
export class TalentFilterStore implements FacetStore {
  #url: UrlSyncedState<TalentFilters>;

  constructor(initial?: URLSearchParams) {
    this.#url = new UrlSyncedState<TalentFilters>(initial ?? new URLSearchParams(), {
      parse: talentFiltersFromParams,
      serialize: talentFiltersToParams,
    });
  }

  /** Live filters — bind inputs to this. */
  get value(): TalentFilters {
    return this.#url.value;
  }

  /** Debounced filters — drive the list reload off this. */
  get applied(): TalentFilters {
    return this.#url.applied;
  }

  /** The filters as they stand in the address bar — build links off this, not `page.url`. */
  get params(): URLSearchParams {
    return this.#url.params;
  }

  get active(): number {
    return activeTalentFilterCount(this.#url.value);
  }

  // Catalogue facets are include-only: there is no `_exclude` on this endpoint, so the
  // selection maps its value set to `include` and leaves `exclude` empty.
  facet(param: string): FacetSelection {
    return { include: this.#url.value.facets[param] ?? [], exclude: [], matchAll: false };
  }

  // The catalogue never excludes, so both the pills' `cycle` (off → include → exclude →
  // off) and the select's `pick` collapse to the same plain include toggle. Same shape
  // the company store has, and for the same reason.
  cycle(param: string, v: string) {
    this.#url.setNow(toggleTalentFacet(this.#url.value, param, v));
  }

  pick(param: string, v: string) {
    this.#url.setNow(toggleTalentFacet(this.#url.value, param, v));
  }

  add(param: string, raw: string) {
    this.#url.setNow(addTalentFacet(this.#url.value, param, raw));
  }

  remove(param: string, v: string) {
    this.#url.setNow(removeTalentFacet(this.#url.value, param, v));
  }

  clearFacet(param: string) {
    this.#url.setNow(clearTalentFacet(this.#url.value, param));
  }

  // The catalogue endpoint has no exclude and no AND/OR mode, and no catalogue facet opts
  // into either, so these are inert — present only to satisfy the FacetStore contract.
  // That is now the second store to need neither; worth noticing, not worth generalising
  // over two examples.
  toggleSign() {}
  setMatchAll() {}

  /** Years is a THRESHOLD, not a set: one value or none, so `undefined` clears it. It has
   *  no place in FacetStore, which is about value sets — the control calls this directly. */
  setMinYears(years: number | undefined) {
    this.#url.setNow(setTalentMinYears(this.#url.value, years));
  }

  clear() {
    this.#url.setNow(emptyTalentFilters());
  }

  /** Replace the entire filter state from a query string and mirror it to the URL — the
   *  commit target for the deferred filter modal (mirrors FilterStore.apply). */
  apply(query: string) {
    this.#url.setNow(talentFiltersFromParams(new URLSearchParams(query)));
  }

  syncFromUrl() {
    this.#url.syncFromUrl();
  }

  dispose() {
    this.#url.dispose();
  }
}
