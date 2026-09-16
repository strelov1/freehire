import { describe, it, expect } from 'vitest';
import { filtersFromProfile, filtersToParams, emptyFilters } from './facetModel';
import type { LocationPreferences, UserProfile } from './types';

// Build a UserProfile with just the fields the seeder reads.
function mkProfile(
  specializations: string[],
  skills: string[],
  location: LocationPreferences | null = null,
  excludedSkills: string[] = [],
  excludedSources: string[] = [],
): UserProfile {
  return {
    specializations,
    skills,
    seniorities: [],
    excluded_skills: excludedSkills,
    excluded_sources: excludedSources,
    excluded_companies: [],
    location_preferences: location,
    derived_location: null,
    cv: null,
    created_at: null,
    updated_at: null,
  };
}

// filtersFromProfile is the pure reset-and-seed used by "Apply my profile": from a
// clean slate it stages the user's profile specializations as `category` values, skills
// as `skills` values, and (when present) the location block as the location facets.
// StagedFilters.applyProfile is a thin wrapper over it.
describe('filtersFromProfile', () => {
  it('seeds category from specializations and skills from skills, and nothing else', () => {
    const f = filtersFromProfile(mkProfile(['backend', 'devops'], ['go', 'kubernetes']));
    const p = filtersToParams(f);
    expect(p.getAll('category')).toEqual(['backend,devops']);
    expect(p.getAll('skills')).toEqual(['go,kubernetes']);
    // No other facet or field leaks in.
    expect([...p.keys()].sort()).toEqual(['category', 'skills']);
  });

  it('starts from a clean slate (independent of any prior state)', () => {
    const f = filtersFromProfile(mkProfile(['frontend'], ['react']));
    // Everything outside the two seeded facets matches an empty filter set.
    const empty = emptyFilters();
    expect(f.q).toEqual(empty.q);
    expect(f.visa).toEqual(empty.visa);
    expect(f.salaryMin).toEqual(empty.salaryMin);
    expect(f.postedWithinDays).toEqual(empty.postedWithinDays);
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
    expect(p.getAll('work_mode')).toEqual(['remote,onsite']);
    // regions = remote ∪ relocation targets.
    expect(p.getAll('regions')).toEqual(['latam,eu']);
    // countries = remote ∪ base ∪ relocation; base 'br' dedupes against remote 'br'.
    expect(p.getAll('countries')).toEqual(['br']);
    // cities = base ∪ relocation targets.
    expect(p.getAll('cities')).toEqual(['Florianópolis,Berlin']);
    // open to relocation → both relocation-supporting values.
    expect(p.getAll('relocation')).toEqual(['supported,required']);
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

  it('seeds the skills exclude set from excluded_skills', () => {
    const f = filtersFromProfile(mkProfile(['backend'], ['go'], null, ['php', 'wordpress']));
    const p = filtersToParams(f);
    expect(p.getAll('skills')).toEqual(['go']);
    expect(p.getAll('skills_exclude')).toEqual(['php,wordpress']);
  });

  it('keeps a skill wanted when it also appears in excluded_skills (include wins)', () => {
    // The backend already drops the overlap, but the seeder must not self-cancel either.
    const f = filtersFromProfile(mkProfile(['backend'], ['go'], null, ['go', 'php']));
    const p = filtersToParams(f);
    expect(p.getAll('skills')).toEqual(['go']);
    expect(p.getAll('skills_exclude')).toEqual(['php']);
  });

  // `base` is where the user LIVES, not where they want the work. For someone who
  // accepts only remote work those are different places, and seeding their home country
  // as a job-country filter would narrow their search to the one country they least need
  // the job to be in. The gate used to be implicit — the profile form only ever collected
  // `base` from on-site/hybrid users — so un-gating that form makes it load-bearing here.
  it('does not seed countries or cities from base for a remote-only user', () => {
    const location: LocationPreferences = {
      work_modes: ['remote'],
      remote: { regions: ['latam'] },
      base: { country: 'co', city: 'Manizales' },
      relocation: { open: false },
    };
    const p = filtersToParams(filtersFromProfile(mkProfile(['backend'], ['go'], location)));
    expect(p.getAll('work_mode')).toEqual(['remote']);
    expect(p.getAll('regions')).toEqual(['latam']);
    expect(p.getAll('countries')).toEqual([]);
    expect(p.getAll('cities')).toEqual([]);
  });

  // Someone who accepts physical work does need the job near where they are, so for them
  // the two coincide and the contribution is kept.
  it('still seeds countries and cities from base for a hybrid user', () => {
    const location: LocationPreferences = {
      work_modes: ['hybrid'],
      remote: { regions: [] },
      base: { country: 'co', city: 'Manizales' },
      relocation: { open: false },
    };
    const p = filtersToParams(filtersFromProfile(mkProfile(['backend'], ['go'], location)));
    expect(p.getAll('countries')).toEqual(['co']);
    expect(p.getAll('cities')).toEqual(['Manizales']);
  });

  it('a profile with no location block seeds only category and skills', () => {
    const p = filtersToParams(filtersFromProfile(mkProfile(['backend'], ['go'], null)));
    expect(p.getAll('category')).toEqual(['backend']);
    expect(p.getAll('skills')).toEqual(['go']);
    for (const param of ['work_mode', 'regions', 'countries', 'cities', 'relocation']) {
      expect(p.getAll(param)).toEqual([]);
    }
  });

  // A profile may hold up to 200 skills and, independently, up to 200 excluded skills,
  // each up to 64 characters (userprofile.go's own caps — free text, no dictionary
  // check), which would blow well past the saved-search query length limit long before
  // the rest of the filter got a chance to matter (freehire "query is too long" bug: a
  // rich profile's own "notify me about jobs matching my profile" toggle failing against
  // its own server-side query bound). filtersFromProfile must cap what it feeds into the
  // query regardless of how many or how long the stored values are.
  describe('excluded sources', () => {
    it('stages an avoided source as an EXCLUDED source facet value', () => {
      // The profile has held excluded_sources since migration 0167, and until now nothing
      // read them: the list was editable in the profile and changed no result anywhere.
      // Seeding here is what makes both that card and the job page's Avoid button true,
      // and it reaches the saved-search alert too, since profileAlertSync builds from the
      // same function.
      const f = filtersFromProfile(mkProfile([], [], null, [], ['smartrecruiters', 'adzuna']));
      const p = filtersToParams(f);

      expect(p.getAll('source_exclude')).toEqual(['smartrecruiters,adzuna']);
      expect(p.getAll('source')).toEqual([]);
    });

    it('seeds nothing when the profile avoids no source', () => {
      const p = filtersToParams(filtersFromProfile(mkProfile(['backend'], [])));

      expect([...p.keys()]).not.toContain('source_exclude');
    });

    it('bounds the contribution so a maxed-out list cannot break the alert query', () => {
      // Same hazard charBudget exists for, and the same cap behind it: a profile may
      // hold up to 200 excluded sources (maxExcludedCount == maxSkills). Fed through whole
      // they would push the profile-derived query past savedsearch's maxQueryLen and the
      // alert toggle would fail with a raw "query is too long".
      const many = Array.from({ length: 200 }, (_, i) => `source-number-${i}`);

      const p = filtersToParams(filtersFromProfile(mkProfile([], [], null, [], many)));
      const kept = p.getAll('source_exclude')[0]?.split(',') ?? [];

      expect(kept.length).toBeGreaterThan(0);
      expect(kept.length).toBeLessThan(many.length);
      // Whole values only — a truncated source key would filter on a source that does not exist.
      expect(kept.every((v) => many.includes(v))).toBe(true);
    });
  });

  describe('bounding the skills contribution', () => {
    // Longer than any real dictionary skill name, to model the free-text worst case
    // rather than the ~8-character average a real pick from the skill picker produces.
    const longSkills = (prefix: string, count: number) =>
      Array.from({ length: count }, (_, i) => `${prefix}-${String(i).padStart(50, '0')}`);

    it('keeps the query well under the server-side length limit for a maximally sized profile', () => {
      const location: LocationPreferences = {
        work_modes: ['remote', 'onsite', 'hybrid'],
        remote: { regions: ['latam', 'eu', 'apac'], countries: Array.from({ length: 20 }, (_, i) => `c${i}`) },
        base: { country: 'br', city: 'Florianópolis' },
        relocation: {
          open: true,
          regions: ['africa', 'mena'],
          countries: Array.from({ length: 20 }, (_, i) => `r${i}`),
          cities: Array.from({ length: 10 }, (_, i) => `City${i}`),
        },
      };
      const profile = mkProfile(
        ['backend', 'devops'],
        longSkills('skill', 200),
        location,
        longSkills('avoid', 200),
      );
      const query = filtersToParams(filtersFromProfile(profile)).toString();
      // The saved-search service's own bound (internal/search/savedsearch/savedsearch.go);
      // duplicated as a literal because this module stays free of any Go/API import.
      expect(query.length).toBeLessThan(4000);
    });

    it('truncates skills as a prefix of the profile’s own order, not a scattered subset', () => {
      const skills = longSkills('skill', 200);
      const p = filtersToParams(filtersFromProfile(mkProfile([], skills)));
      const kept = p.getAll('skills')[0]?.split(',') ?? [];
      expect(kept.length).toBeGreaterThan(0);
      expect(kept.length).toBeLessThan(skills.length);
      expect(skills.slice(0, kept.length)).toEqual(kept);
    });

    it('truncates excluded skills independently of included skills', () => {
      const excluded = longSkills('avoid', 200);
      const p = filtersToParams(filtersFromProfile(mkProfile([], ['go'], null, excluded)));
      const kept = p.getAll('skills_exclude')[0]?.split(',') ?? [];
      expect(kept.length).toBeGreaterThan(0);
      expect(kept.length).toBeLessThan(excluded.length);
      expect(p.getAll('skills')).toEqual(['go']);
    });

    it('leaves an ordinary profile’s skills untouched', () => {
      const skills = ['go', 'kubernetes', 'terraform', 'postgres'];
      const p = filtersToParams(filtersFromProfile(mkProfile([], skills)));
      expect(p.getAll('skills')).toEqual([skills.join(',')]);
    });
  });
});
