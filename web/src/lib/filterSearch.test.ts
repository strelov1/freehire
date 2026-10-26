import { describe, expect, it } from 'vitest';
import { COMPANY_FACETS, TALENT_FACETS } from './facets';
import { COMPANY_RAIL_GROUPS, TALENT_RAIL_GROUPS } from './filterSections';
import { FILTER_SEARCH_LIMIT, jobFilterSearchItems, matchFilterSearch, railFacetSearchItems } from './filterSearch';

describe('matchFilterSearch', () => {
  const items = jobFilterSearchItems({ openWithin: true });

  it('waits for two characters', () => {
    expect(matchFilterSearch(items, '')).toEqual([]);
    expect(matchFilterSearch(items, 'v')).toEqual([]);
  });

  it('finds a specialization by its label and by an alias', () => {
    const byLabel = matchFilterSearch(items, 'backend');
    expect(byLabel.some((h) => h.param === 'category' && h.value === 'backend' && h.group === 'Engineering')).toBe(
      true,
    );
    expect(
      matchFilterSearch(items, 'data engineer').some((h) => h.param === 'category' && h.value === 'data_engineering'),
    ).toBe(true);
  });

  it('accepts the same typos as the facet picker', () => {
    expect(matchFilterSearch(items, 'sineor').some((h) => h.value === 'senior')).toBe(true);
    expect(matchFilterSearch(items, 'backedn').some((h) => h.value === 'backend')).toBe(true);
    expect(matchFilterSearch(items, 'javasript').some((h) => h.value === 'javascript')).toBe(true);
  });

  it('finds skills, visa, and a posted preset', () => {
    expect(matchFilterSearch(items, 'react').some((h) => h.param === 'skills' && h.value === 'react')).toBe(true);
    expect(matchFilterSearch(items, 'visa').some((h) => h.action === 'visa')).toBe(true);
    expect(matchFilterSearch(items, 'posted within 1 week').some((h) => h.action === 'posted:7')).toBe(true);
    expect(matchFilterSearch(items, 'open within 1 week').some((h) => h.action === 'open:7')).toBe(true);
  });

  it('caps the list and keeps ids unique', () => {
    expect(matchFilterSearch(items, 'en').length).toBeLessThanOrEqual(FILTER_SEARCH_LIMIT);
    const ids = items.map((hit) => hit.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

describe('jobFilterSearchItems', () => {
  it('hides panes and params the modal is not showing', () => {
    const salary = jobFilterSearchItems({ railKeys: ['salary'] });
    expect(salary.length).toBeGreaterThan(0);
    expect(salary.every((h) => h.railKey === 'salary')).toBe(true);
    expect(jobFilterSearchItems({ exclude: ['seniority'] }).some((h) => h.param === 'seniority')).toBe(false);
    expect(jobFilterSearchItems().some((h) => h.action?.startsWith('open:'))).toBe(false);
  });
});

describe('railFacetSearchItems', () => {
  it('searches company and talent facets through their rails', () => {
    const companies = railFacetSearchItems(COMPANY_FACETS, COMPANY_RAIL_GROUPS);
    expect(matchFilterSearch(companies, 'startup').some((h) => h.value === 'startup')).toBe(true);
    const talent = railFacetSearchItems(TALENT_FACETS, TALENT_RAIL_GROUPS, { skills: true });
    expect(matchFilterSearch(talent, 'react').some((h) => h.param === 'skills' && h.value === 'react')).toBe(true);
    expect(matchFilterSearch(talent, 'grade').some((h) => h.railKey === 'seniorities')).toBe(true);
  });
});
