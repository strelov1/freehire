import { describe, it, expect } from 'vitest';
import { filtersFromProfile, filtersToParams, emptyFilters } from './facetModel';

// filtersFromProfile is the pure reset-and-seed used by "Apply my profile": from a
// clean slate it stages the user's profile specializations as `category` values and
// skills as `skills` values, and nothing else. StagedFilters.applyProfile is a thin
// wrapper over it.
describe('filtersFromProfile', () => {
  it('seeds category from specializations and skills from skills, and nothing else', () => {
    const f = filtersFromProfile(['backend', 'devops'], ['go', 'kubernetes']);
    const p = filtersToParams(f);
    expect(p.getAll('category')).toEqual(['backend', 'devops']);
    expect(p.getAll('skills')).toEqual(['go', 'kubernetes']);
    // No other facet or field leaks in.
    expect([...p.keys()].sort()).toEqual(['category', 'category', 'skills', 'skills'].sort());
  });

  it('starts from a clean slate (independent of any prior state)', () => {
    const f = filtersFromProfile(['frontend'], ['react']);
    // Everything outside the two seeded facets matches an empty filter set.
    const empty = emptyFilters();
    expect(f.q).toEqual(empty.q);
    expect(f.visa).toEqual(empty.visa);
    expect(f.salaryMin).toEqual(empty.salaryMin);
    expect(f.postedWithinDays).toEqual(empty.postedWithinDays);
    expect(f.sort).toEqual(empty.sort);
  });

  it('trims and dedupes seeded values, and drops empties', () => {
    const f = filtersFromProfile([' backend ', 'backend', ''], ['go', 'go', '  ']);
    const p = filtersToParams(f);
    expect(p.getAll('category')).toEqual(['backend']);
    expect(p.getAll('skills')).toEqual(['go']);
  });

  it('empty inputs yield an empty filter set', () => {
    const p = filtersToParams(filtersFromProfile([], []));
    expect([...p.keys()]).toEqual([]);
  });
});
