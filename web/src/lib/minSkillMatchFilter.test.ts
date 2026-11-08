import { describe, expect, it } from 'vitest';
import {
  activeFilterCount,
  canonicalQuery,
  emptyFilters,
  filtersFromParams,
  filtersToParams,
  queryNarrows,
  savedSearchQuery,
} from './facetModel.js';

// "Minimum skill match" rides in the filters so a saved search restores it (freehire#4095).
describe('min_skill_match', () => {
  it('is absent by default', () => {
    expect(filtersToParams(emptyFilters()).has('min_skill_match')).toBe(false);
  });

  it('round-trips through the query', () => {
    const f = emptyFilters();
    f.minSkillMatch = 70;
    const p = filtersToParams(f);
    expect(p.get('min_skill_match')).toBe('70');
    expect(filtersFromParams(p).minSkillMatch).toBe(70);
    expect(canonicalQuery('min_skill_match=70')).toBe('min_skill_match=70');
  });

  it.each(['0', '101', 'abc', '-5', ''])('reads %j as no threshold', (raw) => {
    expect(filtersFromParams(new URLSearchParams({ min_skill_match: raw })).minSkillMatch).toBeNull();
  });

  it('survives the saved-search query', () => {
    const f = emptyFilters();
    f.minSkillMatch = 70;
    expect(filtersFromParams(new URLSearchParams(savedSearchQuery(f))).minSkillMatch).toBe(70);
  });

  it('counts as an active filter', () => {
    expect(activeFilterCount(filtersFromParams(new URLSearchParams('min_skill_match=70')))).toBe(1);
  });

  // The server refuses a subscription to a query that does not narrow (search.Narrows).
  it.each([
    ['min_skill_match=70', false],
    ['', false],
    ['hide_seen=true', false],
    ['q=go', true],
    ['seniority=senior&min_skill_match=70', true],
  ])('queryNarrows(%j) is %j', (query, want) => {
    expect(queryNarrows(query)).toBe(want);
  });
});
