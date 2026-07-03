// The pure company-catalog filter model: the filter type, its URL <-> state
// (de)serialization, and the pure facet mutators. No SvelteKit or Svelte runes
// here, so this module is unit-testable in plain Node and importable from both the
// reactive store (companyFilters.ts) and the staged-edit surface
// (stagedCompanyFilters.svelte.ts). Mirrors facetModel.ts for the job filters.
//
// Param names match GET /api/v1/companies (`q` plus one repeatable param per
// facet). Unlike the job model there are no `_exclude`/`_mode` conventions — the
// companies endpoint filters by plain array overlap (OR within a facet, AND across
// facets), so a facet is just a value set.

import { COMPANY_FACETS } from './facets';

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

// ---- pure facet mutators (CompanyFilters -> CompanyFilters) ----

function withFacet(f: CompanyFilters, param: string, values: string[]): CompanyFilters {
  return { ...f, facets: { ...f.facets, [param]: values } };
}

/** Add the value if absent, remove it if present. Companies never exclude, so this
 *  is the single interaction behind both the pills (`cycle`) and select (`pick`). */
export function toggleCompanyFacet(f: CompanyFilters, param: string, v: string): CompanyFilters {
  const values = f.facets[param] ?? [];
  return withFacet(f, param, values.includes(v) ? values.filter((x) => x !== v) : [...values, v]);
}

/** Token-input add: put a value into the set; no-op on blank or a duplicate. */
export function addCompanyFacet(f: CompanyFilters, param: string, raw: string): CompanyFilters {
  const v = raw.trim();
  const values = f.facets[param] ?? [];
  if (!v || values.includes(v)) return f;
  return withFacet(f, param, [...values, v]);
}

/** Remove a value from the facet. */
export function removeCompanyFacet(f: CompanyFilters, param: string, v: string): CompanyFilters {
  return withFacet(f, param, (f.facets[param] ?? []).filter((x) => x !== v));
}

/** Reset a single facet. */
export function clearCompanyFacet(f: CompanyFilters, param: string): CompanyFilters {
  return withFacet(f, param, []);
}
