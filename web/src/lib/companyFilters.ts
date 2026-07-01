// Company-catalog filters: the model, its URL <-> state (de)serialization, and a
// reactive store mirrored into the URL query so a filtered view survives reloads,
// sharing, and back/forward. Param names match GET /api/v1/companies (`q` plus one
// repeatable param per facet). Unlike the job FilterStore there are no
// `_exclude`/`_mode` conventions — the companies endpoint filters by plain array
// overlap (OR within a facet, AND across facets), so a facet is just a value set.

import { COMPANY_FACETS, type FacetSelection, type FacetStore } from './facets';
import { UrlSyncedState } from './urlSynced.svelte';

export interface CompanyFilters {
  q: string;
  /** Selected values keyed by facet param (see COMPANY_FACETS). */
  facets: Record<string, string[]>;
}

function emptyFacets(): Record<string, string[]> {
  const out: Record<string, string[]> = {};
  for (const f of COMPANY_FACETS) out[f.param] = [];
  return out;
}

export function emptyCompanyFilters(): CompanyFilters {
  return { q: '', facets: emptyFacets() };
}

/** Serialize to the query shape GET /api/v1/companies reads. */
export function companyFiltersToParams(f: CompanyFilters): URLSearchParams {
  const p = new URLSearchParams();
  if (f.q) p.set('q', f.q);
  for (const def of COMPANY_FACETS) {
    for (const v of f.facets[def.param] ?? []) p.append(def.param, v);
  }
  return p;
}

/** Parse back from URL params. Facet values are a set, so duplicates from a
 *  shared/edited link are collapsed (the keyed {#each} in the controls throws on a
 *  repeat), mirroring the job filters' guard. */
export function companyFiltersFromParams(p: URLSearchParams): CompanyFilters {
  const f = emptyCompanyFilters();
  f.q = p.get('q') ?? '';
  for (const def of COMPANY_FACETS) {
    const values = [...new Set(p.getAll(def.param))];
    if (values.length > 0) f.facets[def.param] = values;
  }
  return f;
}

/** Total selected facet values — drives the mobile "Filters (N)" badge. */
export function activeCompanyFilterCount(f: CompanyFilters): number {
  let n = 0;
  for (const def of COMPANY_FACETS) n += f.facets[def.param]?.length ?? 0;
  return n;
}

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

  facet(param: string): FacetSelection {
    return { values: this.#url.value.facets[param] ?? [], exclude: false, matchAll: false };
  }

  // Free text debounces the reload (setSoon); the URL still updates synchronously.
  setQuery(q: string) {
    this.#url.setSoon({ ...this.#url.value, q });
  }

  /** Add the value to a facet if absent, remove it if present (pills/select). */
  toggle(param: string, v: string) {
    const values = this.facet(param).values;
    const next = values.includes(v) ? values.filter((x) => x !== v) : [...values, v];
    this.#setFacet(param, next);
  }

  add(param: string, raw: string) {
    const v = raw.trim();
    const values = this.facet(param).values;
    if (!v || values.includes(v)) return;
    this.#setFacet(param, [...values, v]);
  }

  remove(param: string, v: string) {
    this.#setFacet(param, this.facet(param).values.filter((x) => x !== v));
  }

  clearFacet(param: string) {
    this.#setFacet(param, []);
  }

  // The companies endpoint has no exclude/AND-OR modes and no company facet opts
  // into them, so these are inert — present only to satisfy the FacetStore contract.
  setExclude() {}
  setMatchAll() {}

  clear() {
    this.#url.setNow(emptyCompanyFilters());
  }

  syncFromUrl() {
    this.#url.syncFromUrl();
  }

  dispose() {
    this.#url.dispose();
  }

  #setFacet(param: string, values: string[]) {
    this.#url.setNow({ ...this.#url.value, facets: { ...this.#url.value.facets, [param]: values } });
  }
}
