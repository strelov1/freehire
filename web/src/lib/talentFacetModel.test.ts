import { describe, expect, it } from 'vitest';

import {
  MAX_FILTER_TERMS,
  MAX_LIMIT,
  activeTalentFilterCount,
  addTalentFacet,
  clearTalentFacet,
  emptyTalentFilters,
  removeTalentFacet,
  setTalentMinYears,
  talentFiltersFromParams,
  talentFiltersToParams,
  toggleTalentFacet,
} from './talentFacetModel';

const read = (search: string) => talentFiltersFromParams(new URLSearchParams(search));
const write = (f: Parameters<typeof talentFiltersToParams>[0]) => talentFiltersToParams(f).toString();

describe('talentFiltersFromParams', () => {
  it('reads every facet and scalar', () => {
    expect(
      read(
        'specializations=ml_ai&categories=backend,devops&seniorities=senior&skills=go,postgresql&tz=Europe&cities=berlin&min_years=5&limit=10&offset=20',
      ),
    ).toEqual({
      facets: {
        specializations: ['ml_ai'],
        categories: ['backend', 'devops'],
        seniorities: ['senior'],
        skills: ['go', 'postgresql'],
        tz: ['Europe'],
        cities: ['berlin'],
      },
      minYears: 5,
      limit: 10,
      offset: 20,
    });
  });

  it('treats an absent facet and an empty one identically', () => {
    expect(read('skills=&cities=,,&tz=%20')).toEqual(emptyTalentFilters());
  });

  it('trims and drops blank terms', () => {
    expect(read('skills=%20go%20,,%20postgresql%20,').facets.skills).toEqual(['go', 'postgresql']);
  });

  it('caps a facet list rather than refusing it', () => {
    const many = Array.from({ length: MAX_FILTER_TERMS + 10 }, (_, i) => `s${i}`).join(',');
    expect(read(`skills=${many}`).facets.skills).toHaveLength(MAX_FILTER_TERMS);
  });

  // Omitted, not clamped. The API answers a value it cannot read by widening the answer
  // and saying so in meta.ignored_params — forwarding one would show an unfiltered
  // catalogue behind chips that claim otherwise.
  it('omits unusable scalars so the API applies its own default', () => {
    expect(read('limit=0').limit).toBeUndefined();
    expect(read(`limit=${MAX_LIMIT + 1}`).limit).toBeUndefined();
    expect(read('limit=12abc').limit).toBeUndefined();
    expect(read('offset=-1').offset).toBeUndefined();
    expect(read('min_years=lots').minYears).toBeUndefined();
    // The ceiling mirrors the Go one exactly; a tighter bound here would drop a filter
    // the API would have read.
    expect(read('min_years=60').minYears).toBe(60);
    expect(read('min_years=61').minYears).toBeUndefined();
  });
});

describe('talentFiltersToParams', () => {
  it('omits a cleared facet rather than writing it empty', () => {
    expect(write({ facets: { skills: [], cities: ['berlin'] } })).toBe('cities=berlin');
  });

  it('omits a zero offset, which is the default', () => {
    expect(write({ facets: {}, offset: 0 })).toBe('');
    expect(write({ facets: {}, offset: 24 })).toBe('offset=24');
  });

  // Two filters selecting the same thing must serialise identically, whatever order the
  // values were added in — that is what makes a shared link stable.
  it('writes facets in the vocabulary order, not insertion order', () => {
    const a = write({ facets: { skills: ['go'], categories: ['backend'] } });
    const b = write({ facets: { categories: ['backend'], skills: ['go'] } });
    expect(a).toBe(b);
    expect(a).toBe('categories=backend&skills=go');
  });

  it('round-trips through talentFiltersFromParams', () => {
    const f = { facets: { skills: ['go', 'postgresql'], tz: ['Europe'] }, minYears: 5, limit: 24, offset: 48 };
    expect(read(write(f))).toEqual(f);
  });
});

describe('mutators', () => {
  it('toggles a value on and off', () => {
    const on = toggleTalentFacet(emptyTalentFilters(), 'skills', 'go');
    expect(on.facets.skills).toEqual(['go']);
    expect(toggleTalentFacet(on, 'skills', 'go').facets.skills).toBeUndefined();
  });

  // Narrowing means "show me these", not "show me page four of these" — an offset kept
  // across a filter change lands on an empty page that looks like no matches.
  it('resets paging on every facet change', () => {
    const f = { facets: { skills: ['go'] }, offset: 48 };
    expect(toggleTalentFacet(f, 'skills', 'kubernetes').offset).toBe(0);
    expect(removeTalentFacet(f, 'skills', 'go').offset).toBe(0);
    expect(clearTalentFacet(f, 'skills').offset).toBe(0);
    expect(setTalentMinYears(f, 5).offset).toBe(0);
  });

  it('ignores a blank or duplicate add, and caps the set', () => {
    const f = toggleTalentFacet(emptyTalentFilters(), 'cities', 'berlin');
    expect(addTalentFacet(f, 'cities', '  ').facets.cities).toEqual(['berlin']);
    expect(addTalentFacet(f, 'cities', 'berlin').facets.cities).toEqual(['berlin']);
    expect(addTalentFacet(f, 'cities', ' lisbon ').facets.cities).toEqual(['berlin', 'lisbon']);

    const full = { facets: { skills: Array.from({ length: MAX_FILTER_TERMS }, (_, i) => `s${i}`) } };
    expect(addTalentFacet(full, 'skills', 'one-too-many').facets.skills).toHaveLength(MAX_FILTER_TERMS);
  });

  it('clears the years threshold with undefined', () => {
    const f = setTalentMinYears(emptyTalentFilters(), 5);
    expect(f.minYears).toBe(5);
    expect(setTalentMinYears(f, undefined).minYears).toBeUndefined();
  });

  it('leaves the input alone', () => {
    const f = emptyTalentFilters();
    toggleTalentFacet(f, 'skills', 'go');
    expect(f.facets).toEqual({});
  });
});

describe('activeTalentFilterCount', () => {
  it('counts facet values and the years threshold, but not paging', () => {
    expect(activeTalentFilterCount({ facets: {}, limit: 24, offset: 48 })).toBe(0);
    expect(activeTalentFilterCount({ facets: { skills: ['go', 'rust'], tz: ['Europe'] } })).toBe(3);
    expect(activeTalentFilterCount({ facets: { skills: ['go'] }, minYears: 5 })).toBe(2);
  });
});
