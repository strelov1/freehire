import { describe, it, expect } from 'vitest';
import { suggestRoles } from './roleSuggest';
import type { FacetCounts } from './types';

/** A facet distribution carrying just the roles a test cares about. Every role a
 *  test expects back needs an entry: a role absent from a PRESENT distribution has
 *  no open jobs and is deliberately not offered. */
const counts = (role: Record<string, number>): FacetCounts => ({
  total: 0,
  facets: { role },
  stats: {},
});

const slugs = (query: string, dist: Record<string, number>): string[] =>
  suggestRoles(query, counts(dist)).map((s) => s.slug);

describe('suggestRoles matching', () => {
  it('matches a role by its display label', () => {
    expect(slugs('data analyst', { data_analytics: 12480 })).toContain('data_analytics');
  });

  it('matches a role by a curated alias', () => {
    expect(slugs('swe', { software_engineer: 78421 })).toContain('software_engineer');
  });

  it('matches a role the query only prefixes', () => {
    expect(slugs('data an', { data_analytics: 12480 })).toContain('data_analytics');
  });

  it('offers a role once even when several of its aliases match', () => {
    // "agent engineer" and "ai agent engineer" are both aliases of ai_engineering.
    const found = slugs('agent engineer', { ai_engineering: 5000 });
    expect(found.filter((s) => s === 'ai_engineering')).toHaveLength(1);
  });
});

describe('suggestRoles ranking', () => {
  // Both ranking tests deliberately contradict the catalogue's own order
  // (ROLE_LABELS is keyed alphabetically by slug), so neither can pass on
  // unsorted output: `data_analytics` precedes `data_engineering` there, and
  // `design` precedes `design_engineer`.
  it('ranks the role with more open vacancies first', () => {
    const found = slugs('data', { data_analytics: 9210, data_engineering: 12480 });
    expect(found.indexOf('data_engineering')).toBeLessThan(found.indexOf('data_analytics'));
  });

  it('carries each role open-vacancy count', () => {
    const [top] = suggestRoles('data analyst', counts({ data_analytics: 12480 }));
    expect(top).toMatchObject({ slug: 'data_analytics', label: 'Data Analyst', count: 12480 });
  });

  it('breaks a tie on label, not on catalogue order', () => {
    // 'Design Engineer' sorts before 'Designer', while slug `design` sorts before
    // `design_engineer` — so equal counts must invert the catalogue's order.
    const dist = { design: 500, design_engineer: 500 };
    const found = slugs('design', dist);
    expect(found.indexOf('design_engineer')).toBeLessThan(found.indexOf('design'));
    expect(found).toEqual(slugs('design', dist));
  });

});

describe('suggestRoles when a figure is absent', () => {
  it('omits the count entirely while the distribution is unmeasured', () => {
    // Not zero: an unmeasured role must not be drawn as one with no vacancies.
    const [top] = suggestRoles('data analyst', null);
    expect(top).toBeDefined();
    expect(top).not.toHaveProperty('count');
  });

  it('still offers matches while the distribution is unmeasured', () => {
    expect(suggestRoles('data analyst', null).map((s) => s.slug)).toContain('data_analytics');
  });

  it('drops a role the measured distribution has no jobs for', () => {
    // Measured and absent means zero — a suggestion leading to an empty page.
    expect(slugs('data analyst', { data_engineering: 9210 })).not.toContain('data_analytics');
  });

  it('does not re-offer a role already applied to the facet', () => {
    const found = suggestRoles('data analyst', counts({ data_analytics: 12480 }), [
      'data_analytics',
    ]);
    expect(found.map((s) => s.slug)).not.toContain('data_analytics');
  });
});

describe('suggestRoles threshold', () => {
  // The threshold lives here, not in the component: one character matches most of a
  // 1,290-role catalogue, and an empty query matches all of it (fuzzyMatch treats an
  // empty query as matching everything).
  it('offers nothing for a single character', () => {
    expect(suggestRoles('d', counts({ data_analytics: 12480 }))).toEqual([]);
  });

  it('offers nothing for an empty or blank query', () => {
    const dist = counts({ data_analytics: 12480 });
    expect(suggestRoles('', dist)).toEqual([]);
    expect(suggestRoles('   ', dist)).toEqual([]);
  });

  it('offers matches from two characters on', () => {
    expect(slugs('qa', { qa: 60 })).toContain('qa');
  });
});

describe('suggestRoles capping', () => {
  it('offers at most five roles', () => {
    const dist = {
      backend: 9,
      frontend: 8,
      data_engineering: 7,
      security: 6,
      qa: 5,
      embedded: 4,
      hardware: 3,
      sre: 2,
      architecture: 1,
    };
    expect(suggestRoles('engineer', counts(dist))).toHaveLength(5);
  });
});
