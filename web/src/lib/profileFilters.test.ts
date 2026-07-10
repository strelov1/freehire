import { describe, it, expect } from 'vitest';
import { filtersFromProfile, filtersToParams, emptyFilters } from './facetModel';
import type { LocationPreferences, UserProfile } from './types';

// Build a UserProfile with just the fields the seeder reads.
function mkProfile(
  specializations: string[],
  skills: string[],
  location: LocationPreferences | null = null,
): UserProfile {
  return { specializations, skills, location_preferences: location, created_at: null, updated_at: null };
}

// filtersFromProfile is the pure reset-and-seed used by "Apply my profile": from a
// clean slate it stages the user's profile specializations as `category` values, skills
// as `skills` values, and (when present) the location block as the location facets.
// StagedFilters.applyProfile is a thin wrapper over it.
describe('filtersFromProfile', () => {
  it('seeds category from specializations and skills from skills, and nothing else', () => {
    const f = filtersFromProfile(mkProfile(['backend', 'devops'], ['go', 'kubernetes']));
    const p = filtersToParams(f);
    expect(p.getAll('category')).toEqual(['backend', 'devops']);
    expect(p.getAll('skills')).toEqual(['go', 'kubernetes']);
    // No other facet or field leaks in.
    expect([...p.keys()].sort()).toEqual(['category', 'category', 'skills', 'skills'].sort());
  });

  it('starts from a clean slate (independent of any prior state)', () => {
    const f = filtersFromProfile(mkProfile(['frontend'], ['react']));
    // Everything outside the two seeded facets matches an empty filter set.
    const empty = emptyFilters();
    expect(f.q).toEqual(empty.q);
    expect(f.visa).toEqual(empty.visa);
    expect(f.salaryMin).toEqual(empty.salaryMin);
    expect(f.postedWithinDays).toEqual(empty.postedWithinDays);
    expect(f.sort).toEqual(empty.sort);
  });

  it('trims and dedupes seeded values, and drops empties', () => {
    const f = filtersFromProfile(mkProfile([' backend ', 'backend', ''], ['go', 'go', '  ']));
    const p = filtersToParams(f);
    expect(p.getAll('category')).toEqual(['backend']);
    expect(p.getAll('skills')).toEqual(['go']);
  });

  it('empty inputs yield an empty filter set', () => {
    const p = filtersToParams(filtersFromProfile(mkProfile([], [])));
    expect([...p.keys()]).toEqual([]);
  });

  it('flattens the location block: the Florianópolis case', () => {
    const location: LocationPreferences = {
      work_modes: ['remote', 'onsite'],
      remote: { regions: ['latam'], countries: ['br'] },
      base: { country: 'br', city: 'Florianópolis' },
      relocation: { open: true, regions: ['eu'], cities: ['Berlin'] },
    };
    const p = filtersToParams(filtersFromProfile(mkProfile(['backend'], ['go'], location)));
    expect(p.getAll('work_mode')).toEqual(['remote', 'onsite']);
    // regions = remote ∪ relocation targets.
    expect(p.getAll('regions')).toEqual(['latam', 'eu']);
    // countries = remote ∪ base ∪ relocation; base 'br' dedupes against remote 'br'.
    expect(p.getAll('countries')).toEqual(['br']);
    // cities = base ∪ relocation targets.
    expect(p.getAll('cities')).toEqual(['Florianópolis', 'Berlin']);
    // open to relocation → both relocation-supporting values.
    expect(p.getAll('relocation')).toEqual(['supported', 'required']);
  });

  it('seeds no relocation facet when the user is not open to relocating', () => {
    const location: LocationPreferences = {
      remote: { regions: ['latam'] },
      base: {},
      relocation: { open: false, cities: ['Berlin'] },
    };
    const p = filtersToParams(filtersFromProfile(mkProfile(['backend'], ['go'], location)));
    expect(p.getAll('relocation')).toEqual([]);
    // Relocation targets are ignored when not open (open is the gate for the whole block).
    expect(p.getAll('regions')).toEqual(['latam']);
    expect(p.getAll('cities')).toEqual([]);
  });

  it('a profile with no location block seeds only category and skills', () => {
    const p = filtersToParams(filtersFromProfile(mkProfile(['backend'], ['go'], null)));
    expect(p.getAll('category')).toEqual(['backend']);
    expect(p.getAll('skills')).toEqual(['go']);
    for (const param of ['work_mode', 'regions', 'countries', 'cities', 'relocation']) {
      expect(p.getAll(param)).toEqual([]);
    }
  });
});
