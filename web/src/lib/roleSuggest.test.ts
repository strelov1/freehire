import { describe, it, expect } from 'vitest';
import { suggestRoles } from './roleSuggest';
import { baseRole } from './facets';
import roleDistribution from './fixtures/roleDistribution.json';
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

// Every fixture above reduces the 1,290-role catalogue to a handful, because the
// eligibility filter drops whatever the distribution omits. That is what let a
// green suite ship a ranker whose top result was wrong for `backend`, `devops`,
// `frontend` and `designer`. These run against the real catalogue and a real
// production distribution, and assert RANK, not presence.
const live = counts(roleDistribution);
const top = (query: string, dist: FacetCounts | null = live) =>
  suggestRoles(query, dist)[0]?.label;
const labels = (query: string) => suggestRoles(query, live).map((s) => s.label);

describe('suggestRoles against the real catalogue', () => {
  it('leads with the role the query names, not the biggest fuzzy match', () => {
    // Sales Specialist (147k) is reachable from `devops` via its `revops` alias, and
    // Marketing Specialist (56k) from `backend` via `growth hacker`.
    expect(top('devops')).toBe('DevOps Engineer');
    expect(top('backend')).toBe('Backend Engineer');
    expect(top('frontend')).toBe('Frontend Engineer');
    expect(top('designer')).toBe('Designer');
    expect(top('swe')).toBe('Software Engineer');
  });

  it('prefers a role the query names outright over one it cuts a word short in', () => {
    // `database developer` is an alias of Software Generalist (75,427 jobs) and does
    // start with "data" — but only mid-word, while "Data Analyst" (65,371) completes
    // one. Without that distinction the bigger bucket leads a search for "data".
    expect(top('data')).toBe('Data Analyst');
  });

  it('never offers a role reached only by typo tolerance', () => {
    // Marketing Specialist is reachable from `backend` only by edit distance against
    // its `growth hacker` alias. Offering it beside Backend Engineer reads as noise —
    // and on a real typo it LEADS, since nothing else separates two fuzzy hits and it
    // owns the bigger bucket.
    expect(labels('backend')).toEqual(['Backend Engineer']);
    expect(labels('backedn')).toEqual([]);
  });

  it('leads with the named role even before any count is measured', () => {
    // Ordering the unmeasured case by label alone returns five 'C-Level …' rows.
    expect(top('swe', null)).toBe('Software Engineer');
    expect(top('data an', null)).toBe('Data Analyst');
    expect(top('product man', null)).toBe('Product Manager');
  });

  it('collapses a role whose grades would otherwise fill every row', () => {
    // Uncollapsed this returns Data Analyst plus its senior, lead, junior and intern
    // grades — five rows, one role. The query names exactly one role, so one row is
    // the honest answer.
    expect(labels('data analyst')).toEqual(['Data Analyst']);
  });

  it('never offers two grades of the same role', () => {
    const found = suggestRoles('data', live);
    const bases = found.map((s) => baseRole(s.slug));
    expect(found.length).toBeGreaterThan(1);
    expect(new Set(bases).size).toBe(bases.length);
  });

  it('keeps the grade the query actually names', () => {
    expect(top('senior data analyst')).toBe('Senior Data Analyst');
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
