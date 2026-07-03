import { describe, it, expect } from 'vitest';
import {
  emptyFacet,
  emptyFilters,
  filtersToParams,
  filtersFromParams,
  activeFilterCount,
  canonicalQuery,
  signOf,
  facetSetSign,
  facetCycle,
  facetPick,
  facetToggleSign,
  facetAdd,
  facetRemove,
  type FacetState,
  type JobFilters,
} from './facetModel';

// A JobFilters seeded with one facet's state, for serialization tests.
function withSkills(st: Partial<FacetState>): JobFilters {
  const f = emptyFilters();
  f.facets.skills = { include: [], exclude: [], matchAll: false, ...st };
  return f;
}

// The skills facet is always present (emptyFacets seeds every FACET), so this read
// is safe; the helper drops the index-signature `| undefined` for terser asserts.
const sk = (f: JobFilters): FacetState => f.facets.skills!;

describe('emptyFacet', () => {
  it('is two empty sets and OR mode', () => {
    expect(emptyFacet()).toEqual({ include: [], exclude: [], matchAll: false });
  });
});

describe('filtersToParams', () => {
  it('serializes include to the bare param and exclude to <param>_exclude', () => {
    const p = filtersToParams(withSkills({ include: ['nodejs', 'react'], exclude: ['php'] }));
    expect(p.getAll('skills')).toEqual(['nodejs', 'react']);
    expect(p.getAll('skills_exclude')).toEqual(['php']);
  });

  it('emits <param>_mode=and only when matchAll and more than one included value', () => {
    expect(filtersToParams(withSkills({ include: ['go', 'rust'], matchAll: true })).get('skills_mode')).toBe('and');
    expect(filtersToParams(withSkills({ include: ['go'], matchAll: true })).get('skills_mode')).toBeNull();
    expect(filtersToParams(withSkills({ include: ['go', 'rust'], matchAll: false })).get('skills_mode')).toBeNull();
  });

  it('omits a facet with no values', () => {
    expect(filtersToParams(emptyFilters()).toString()).toBe('');
  });
});

describe('filtersFromParams', () => {
  it('reads the bare param into include and <param>_exclude into exclude', () => {
    const f = filtersFromParams(new URLSearchParams('skills=nodejs&skills_exclude=php&skills_exclude=.net'));
    expect(sk(f).include).toEqual(['nodejs']);
    expect(sk(f).exclude).toEqual(['php', '.net']);
  });

  it('reads <param>_mode=and into matchAll', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go&skills=rust&skills_mode=and'));
    expect(sk(f).matchAll).toBe(true);
  });

  it('drops a value from include when it also appears in exclude (exclude wins)', () => {
    const f = filtersFromParams(new URLSearchParams('skills=php&skills_exclude=php'));
    expect(sk(f).include).toEqual([]);
    expect(sk(f).exclude).toEqual(['php']);
  });

  it('de-duplicates repeated URL values', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go&skills=go'));
    expect(sk(f).include).toEqual(['go']);
  });
});

describe('canonicalQuery', () => {
  it('is idempotent for a mixed include/exclude query', () => {
    const q = 'skills=nodejs&skills_exclude=php';
    const once = canonicalQuery(q);
    expect(canonicalQuery(once)).toBe(once);
    expect(new URLSearchParams(once).getAll('skills')).toEqual(['nodejs']);
    expect(new URLSearchParams(once).getAll('skills_exclude')).toEqual(['php']);
  });
});

describe('activeFilterCount', () => {
  it('counts included and excluded values plus scalar filters', () => {
    const f = withSkills({ include: ['a', 'b'], exclude: ['c'] });
    f.visa = true;
    f.salaryMin = 100000;
    expect(activeFilterCount(f)).toBe(5); // 2 include + 1 exclude + visa + salary
  });
});

describe('sign transitions (pure)', () => {
  const inc = (): FacetState => ({ include: ['go'], exclude: [], matchAll: false });

  it('signOf reports the value state', () => {
    expect(signOf(emptyFacet(), 'go')).toBe('off');
    expect(signOf({ include: ['go'], exclude: [], matchAll: false }, 'go')).toBe('include');
    expect(signOf({ include: [], exclude: ['go'], matchAll: false }, 'go')).toBe('exclude');
  });

  it('facetSetSign moves a value between sets and does not mutate the input', () => {
    const st = inc();
    const next = facetSetSign(st, 'go', 'exclude');
    expect(next).toEqual({ include: [], exclude: ['go'], matchAll: false });
    expect(st).toEqual({ include: ['go'], exclude: [], matchAll: false }); // unchanged
    expect(facetSetSign(next, 'go', 'off')).toEqual({ include: [], exclude: [], matchAll: false });
  });

  it('facetCycle goes off -> include -> exclude -> off', () => {
    let st = emptyFacet();
    st = facetCycle(st, 'go');
    expect(signOf(st, 'go')).toBe('include');
    st = facetCycle(st, 'go');
    expect(signOf(st, 'go')).toBe('exclude');
    st = facetCycle(st, 'go');
    expect(signOf(st, 'go')).toBe('off');
  });

  it('facetPick adds to include, and removes when already selected in either set', () => {
    expect(signOf(facetPick(emptyFacet(), 'go'), 'go')).toBe('include');
    expect(signOf(facetPick({ include: ['go'], exclude: [], matchAll: false }, 'go'), 'go')).toBe('off');
    expect(signOf(facetPick({ include: [], exclude: ['go'], matchAll: false }, 'go'), 'go')).toBe('off');
  });

  it('facetToggleSign flips include and exclude', () => {
    expect(signOf(facetToggleSign({ include: ['go'], exclude: [], matchAll: false }, 'go'), 'go')).toBe('exclude');
    expect(signOf(facetToggleSign({ include: [], exclude: ['go'], matchAll: false }, 'go'), 'go')).toBe('include');
  });

  it('facetAdd adds to include, no-op on blank or existing value', () => {
    expect(facetAdd(emptyFacet(), '  go ').include).toEqual(['go']);
    expect(facetAdd(inc(), 'go')).toEqual(inc()); // duplicate
    expect(facetAdd(emptyFacet(), '   ')).toEqual(emptyFacet()); // blank
    expect(facetAdd({ include: [], exclude: ['go'], matchAll: false }, 'go').exclude).toEqual(['go']); // already excluded -> no-op
  });

  it('facetRemove clears the value from both sets', () => {
    expect(facetRemove({ include: ['go'], exclude: [], matchAll: false }, 'go')).toEqual(emptyFacet());
    expect(facetRemove({ include: [], exclude: ['go'], matchAll: false }, 'go')).toEqual(emptyFacet());
  });
});
