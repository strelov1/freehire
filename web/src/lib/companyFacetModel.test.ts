import { describe, expect, test } from 'vitest';
import {
  activeCompanyFilterCount,
  addCompanyFacet,
  clearCompanyFacet,
  companyFiltersFromParams,
  companyFiltersToParams,
  emptyCompanyFilters,
  removeCompanyFacet,
  toggleCompanyFacet,
} from './companyFacetModel';

describe('companyFacetModel serialization', () => {
  test('empty filters serialize to an empty query', () => {
    expect(companyFiltersToParams(emptyCompanyFilters()).toString()).toBe('');
  });

  test('toParams emits q plus one repeatable param per selected value', () => {
    let f = emptyCompanyFilters();
    f = { ...f, q: 'lab' };
    f = toggleCompanyFacet(f, 'regions', 'europe');
    f = toggleCompanyFacet(f, 'regions', 'asia');
    f = toggleCompanyFacet(f, 'company_type', 'startup');
    const p = companyFiltersToParams(f);
    expect(p.get('q')).toBe('lab');
    expect(p.getAll('regions')).toEqual(['europe', 'asia']);
    expect(p.getAll('company_type')).toEqual(['startup']);
  });

  test('fromParams then toParams round-trips', () => {
    const p = new URLSearchParams('q=lab&regions=europe&regions=asia&company_type=startup');
    const f = companyFiltersFromParams(p);
    expect(companyFiltersToParams(f).toString()).toBe(p.toString());
  });

  test('fromParams collapses duplicate values into a set', () => {
    const f = companyFiltersFromParams(new URLSearchParams('regions=europe&regions=europe'));
    expect(f.facets.regions).toEqual(['europe']);
  });
});

describe('companyFacetModel pure mutators', () => {
  test('toggle adds a value when absent and removes it when present', () => {
    let f = emptyCompanyFilters();
    f = toggleCompanyFacet(f, 'regions', 'europe');
    expect(f.facets.regions).toEqual(['europe']);
    f = toggleCompanyFacet(f, 'regions', 'europe');
    expect(f.facets.regions).toEqual([]);
  });

  test('toggle does not mutate the input', () => {
    const f = emptyCompanyFilters();
    const next = toggleCompanyFacet(f, 'regions', 'europe');
    expect(f.facets.regions).toEqual([]);
    expect(next).not.toBe(f);
  });

  test('add trims and ignores blank and duplicate values', () => {
    let f = emptyCompanyFilters();
    f = addCompanyFacet(f, 'countries', ' de ');
    f = addCompanyFacet(f, 'countries', 'de');
    f = addCompanyFacet(f, 'countries', '   ');
    expect(f.facets.countries).toEqual(['de']);
  });

  test('remove drops a value, leaving others', () => {
    let f = emptyCompanyFilters();
    f = toggleCompanyFacet(f, 'countries', 'de');
    f = toggleCompanyFacet(f, 'countries', 'sg');
    f = removeCompanyFacet(f, 'countries', 'de');
    expect(f.facets.countries).toEqual(['sg']);
  });

  test('clearFacet empties only the named facet', () => {
    let f = emptyCompanyFilters();
    f = toggleCompanyFacet(f, 'regions', 'europe');
    f = toggleCompanyFacet(f, 'company_type', 'startup');
    f = clearCompanyFacet(f, 'regions');
    expect(f.facets.regions).toEqual([]);
    expect(f.facets.company_type).toEqual(['startup']);
  });
});

describe('activeCompanyFilterCount', () => {
  test('counts every selected value across facets, ignoring q', () => {
    let f = emptyCompanyFilters();
    f = { ...f, q: 'ignored' };
    f = toggleCompanyFacet(f, 'regions', 'europe');
    f = toggleCompanyFacet(f, 'regions', 'asia');
    f = toggleCompanyFacet(f, 'company_type', 'startup');
    expect(activeCompanyFilterCount(f)).toBe(3);
  });
});

describe('remote_regions facet (curated hiring regions)', () => {
  test('is a selectable facet, distinct from the job-derived regions facet', () => {
    let f = emptyCompanyFilters();
    f = toggleCompanyFacet(f, 'remote_regions', 'eu');
    f = toggleCompanyFacet(f, 'regions', 'north_america');
    expect(f.facets.remote_regions).toEqual(['eu']);
    expect(f.facets.regions).toEqual(['north_america']);
    expect(activeCompanyFilterCount(f)).toBe(2);
  });

  test('round-trips through the URL as a repeatable param', () => {
    const p = new URLSearchParams('remote_regions=eu&remote_regions=apac');
    const f = companyFiltersFromParams(p);
    expect(f.facets.remote_regions).toEqual(['eu', 'apac']);
    expect(companyFiltersToParams(f).getAll('remote_regions')).toEqual(['eu', 'apac']);
  });
});

describe('yc facets (curated YC directory)', () => {
  test('yc_status and yc_batch are selectable and round-trip', () => {
    let f = emptyCompanyFilters();
    f = toggleCompanyFacet(f, 'yc_status', 'Public');
    f = toggleCompanyFacet(f, 'yc_batch', 'Summer 2009');
    expect(f.facets.yc_status).toEqual(['Public']);
    expect(f.facets.yc_batch).toEqual(['Summer 2009']);
    expect(activeCompanyFilterCount(f)).toBe(2);
    const p = companyFiltersToParams(f);
    expect(p.getAll('yc_status')).toEqual(['Public']);
    expect(companyFiltersFromParams(p).facets.yc_batch).toEqual(['Summer 2009']);
  });
});
