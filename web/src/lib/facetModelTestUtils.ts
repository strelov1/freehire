import { emptyFilters, type FacetState, type JobFilters, type JobSort } from './facetModel';

// Builders shared by facetModel's tests.

// A JobFilters carrying a query and an EXPLICITLY chosen ordering — the pair the sort
// default depends on. Pass `null` for "the caller has not chosen one".
export function withQuery(q: string, sort: JobSort | null): JobFilters {
  return { ...emptyFilters(), q, sort };
}

// A JobFilters seeded with one facet's state, for serialization tests.
export function withSkills(st: Partial<FacetState>): JobFilters {
  const f = emptyFilters();
  f.facets.skills = { include: [], exclude: [], matchAll: false, ...st };
  return f;
}
