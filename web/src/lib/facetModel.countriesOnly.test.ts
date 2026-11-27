import { describe, it, expect } from 'vitest';
import {
  emptyFilters,
  filtersToParams,
  filtersFromParams,
  activeFilterCount,
  appliesCountriesOnly,
  COUNTRIES_ONLY_LABEL,
} from './facetModel';
import { savedSearchQuery } from './savedSearchQuery';

const withCountry = (include: string[]) => ({
  ...emptyFilters(),
  facets: { ...emptyFilters().facets, countries: { include, exclude: [], matchAll: false } },
});

describe('countriesOnly', () => {
  // It modifies the countries facet, so it is written and read only beside one — a link
  // carrying it alone must not silently narrow the feed.
  it('round-trips beside an included country and is inert without one', () => {
    expect(filtersToParams(emptyFilters()).has('countries_only')).toBe(false);

    const on = { ...withCountry(['ch']), countriesOnly: true };
    const p = filtersToParams(on);
    expect(p.get('countries_only')).toBe('true');
    expect(filtersFromParams(p).countriesOnly).toBe(true);

    // On, but no country to modify: neither written nor read back.
    expect(filtersToParams({ ...emptyFilters(), countriesOnly: true }).has('countries_only')).toBe(false);
    expect(filtersFromParams(new URLSearchParams('countries_only=true')).countriesOnly).toBe(false);
  });

  it('counts as one active filter only where it applies', () => {
    expect(activeFilterCount({ ...withCountry(['ch']), countriesOnly: true })).toBe(2); // country + modifier
    expect(activeFilterCount({ ...emptyFilters(), countriesOnly: true })).toBe(0);
  });

  it('labels the filter without naming a selection it may not enforce', () => {
    expect(COUNTRIES_ONLY_LABEL).toBe('Only jobs located in one country');
  });

  // The digest matcher replays the stored query, so the alert must mail the same set.
  it('reaches a saved search', () => {
    const q = savedSearchQuery({ ...withCountry(['ch']), countriesOnly: true });
    expect(new URLSearchParams(q).get('countries_only')).toBe('true');
  });

  it('applies only with an included country', () => {
    expect(appliesCountriesOnly({ ...withCountry(['ch']), countriesOnly: true })).toBe(true);
    expect(appliesCountriesOnly({ ...withCountry([]), countriesOnly: true })).toBe(false);
    expect(appliesCountriesOnly({ ...withCountry(['ch']), countriesOnly: false })).toBe(false);
  });
});
