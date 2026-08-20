import { describe, it, expect } from 'vitest';
import {
  emptyFacet,
  emptyFilters,
  filtersToParams,
  filtersFromParams,
  activeFilterCount,
  canonicalQuery,
  savedSearchQuery,
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
import { must } from './utils';

// A JobFilters seeded with one facet's state, for serialization tests.
function withSkills(st: Partial<FacetState>): JobFilters {
  const f = emptyFilters();
  f.facets.skills = { include: [], exclude: [], matchAll: false, ...st };
  return f;
}

// The skills facet is always present (emptyFacets seeds every FACET), so this read
// is safe; the helper drops the index-signature `| undefined` for terser asserts.
const sk = (f: JobFilters): FacetState => must(f.facets.skills);

describe('emptyFacet', () => {
  it('is two empty sets and OR mode', () => {
    expect(emptyFacet()).toEqual({ include: [], exclude: [], matchAll: false });
  });
});

describe('filtersToParams', () => {
  it('serializes include to one comma-joined bare param and exclude to <param>_exclude', () => {
    const p = filtersToParams(withSkills({ include: ['nodejs', 'react'], exclude: ['php', 'java'] }));
    expect(p.getAll('skills')).toEqual(['nodejs,react']);
    expect(p.getAll('skills_exclude')).toEqual(['php,java']);
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

  it('splits a comma-joined value into multiple included values', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go,react'));
    expect(sk(f).include).toEqual(['go', 'react']);
  });

  it('splits a comma-joined value in <param>_exclude too', () => {
    const f = filtersFromParams(new URLSearchParams('skills_exclude=java,cpp'));
    expect(sk(f).exclude).toEqual(['java', 'cpp']);
  });

  it('still parses the old repeated-key form (backward compatibility)', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go&skills=react&skills=aws'));
    expect(sk(f).include).toEqual(['go', 'react', 'aws']);
  });

  it('unions a comma-joined entry with a repeated key for the same param', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go,react&skills=aws'));
    expect(sk(f).include).toEqual(['go', 'react', 'aws']);
  });

  it('drops stray/doubled commas without producing an empty value', () => {
    const f = filtersFromParams(new URLSearchParams('skills=go,,react,'));
    expect(sk(f).include).toEqual(['go', 'react']);
  });
});

describe('filtersToParams / filtersFromParams round-trip', () => {
  it('round-trips a multi-value facet through the new comma-joined serialization', () => {
    const f = withSkills({ include: ['go', 'react', 'aws'], exclude: ['java'] });
    const back = filtersFromParams(filtersToParams(f));
    expect(sk(back).include).toEqual(['go', 'react', 'aws']);
    expect(sk(back).exclude).toEqual(['java']);
  });

  it('round-trips an old-style repeated-key URL to the same filter state as the new form', () => {
    const oldStyle = filtersFromParams(new URLSearchParams('skills=go&skills=react'));
    const newStyle = filtersFromParams(new URLSearchParams('skills=go,react'));
    expect(oldStyle).toEqual(newStyle);
  });
});

describe('experienceYearsMax', () => {
  it('is absent from the URL when unset', () => {
    expect(filtersToParams(emptyFilters()).has('experience_years_max')).toBe(false);
  });

  it('serializes a bound and reads it back', () => {
    const f = emptyFilters();
    f.experienceYearsMax = 3;
    const p = filtersToParams(f);
    expect(p.get('experience_years_max')).toBe('3');
    expect(filtersFromParams(p).experienceYearsMax).toBe(3);
  });

  // Zero is the entry-level selector, not "unset". A falsy guard here would drop the
  // one bound juniors need, and the URL would silently widen back to the whole
  // catalogue while the control still showed the leftmost stop.
  it('serializes a zero bound rather than treating it as unset', () => {
    const f = emptyFilters();
    f.experienceYearsMax = 0;
    const p = filtersToParams(f);
    expect(p.get('experience_years_max')).toBe('0');
    expect(filtersFromParams(p).experienceYearsMax).toBe(0);
  });

  // `Number(' ')` is 0 and `' '` is truthy, so a whitespace-only value slips past a
  // naive presence check and lands on the entry-level filter — a shared or
  // hand-edited link would narrow to almost nothing without saying why.
  it('reads a junk, blank or negative URL value back as no bound', () => {
    for (const raw of ['', ' ', '%20', 'abc', '-1', '2.5']) {
      const back = filtersFromParams(new URLSearchParams(`experience_years_max=${raw}`));
      expect(back.experienceYearsMax, `experience_years_max=${raw}`).toBeNull();
    }
  });

  it('counts as one active filter, including at zero', () => {
    const f = emptyFilters();
    f.experienceYearsMax = 0;
    expect(activeFilterCount(f)).toBe(1);
  });
});

describe('role facet round-trips through the generic param path', () => {
  it('serializes include/exclude/mode and reads them back', () => {
    const f = emptyFilters();
    // role is a registered FACET, so emptyFilters seeds it.
    f.facets.role = { include: ['senior_backend', 'lead_frontend'], exclude: ['fractional_cto'], matchAll: true };

    const p = filtersToParams(f);
    expect(p.getAll('role')).toEqual(['senior_backend,lead_frontend']);
    expect(p.getAll('role_exclude')).toEqual(['fractional_cto']);
    expect(p.get('role_mode')).toBe('and');

    const back = filtersFromParams(p);
    const role = must(back.facets.role);
    expect(role.include).toEqual(['senior_backend', 'lead_frontend']);
    expect(role.exclude).toEqual(['fractional_cto']);
    expect(role.matchAll).toBe(true);
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

// The CV-similarity sort mode (and its `sort=cv` URL param) was removed along with
// /me/recommendations — JobFilters no longer has a `sort` field at all. A
// pre-existing shared `?sort=cv` link must not error; it should read exactly like
// a URL with no `sort` param at all (falls back to the default "Newest" feed).
describe('sort param removal', () => {
  it('ignores a legacy sort=cv param — falls back to the default feed, not an error', () => {
    expect(filtersFromParams(new URLSearchParams('sort=cv'))).toEqual(filtersFromParams(new URLSearchParams('')));
  });

  it('never serializes a sort param', () => {
    expect(filtersToParams(emptyFilters()).get('sort')).toBeNull();
    expect(new URLSearchParams(savedSearchQuery(withSkills({ include: ['go'] }))).get('sort')).toBeNull();
  });
});
