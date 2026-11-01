import { describe, expect, it } from 'vitest';
import { isIndustryMiss, lookupIndustry, searchIndustries, seedDictionaryReturnParams } from './dictionaryIndustry';

describe('lookupIndustry', () => {
  it('returns the canonical slug and display label for a slug or label', () => {
    expect(lookupIndustry('food-and-beverage')).toEqual({
      type: 'canonical',
      slug: 'food-and-beverage',
      label: 'Food & Beverage',
    });
    expect(lookupIndustry('FOOD & BEVERAGE')).toEqual({
      type: 'canonical',
      slug: 'food-and-beverage',
      label: 'Food & Beverage',
    });
  });

  it('matches Go normalization for punctuation, whitespace, and Unicode', () => {
    expect(lookupIndustry('Food&Beverage')).toEqual({
      type: 'canonical',
      slug: 'food-and-beverage',
      label: 'Food & Beverage',
    });
    expect(lookupIndustry('\u00a0 Food\t&  Beverage \u00a0')).toEqual({
      type: 'canonical',
      slug: 'food-and-beverage',
      label: 'Food & Beverage',
    });
    expect(lookupIndustry('İT Services')).toEqual({
      type: 'canonical',
      slug: 'it-services',
      label: 'IT Services',
    });
    expect(searchIndustries('İT Services')).toContainEqual({
      slug: 'it-services',
      label: 'IT Services',
    });
    expect(isIndustryMiss('İT Services')).toBe(false);
  });

  it('resolves an alias without treating it as a missing term', () => {
    expect(lookupIndustry('finance')).toEqual({
      type: 'alias',
      slug: 'financial-services',
      label: 'Financial Services',
    });
  });

  it('marks an unknown term as a miss', () => {
    expect(lookupIndustry('underwater basket weaving')).toEqual({ type: 'miss' });
  });

  it('searches aliases without inventing a company or job count', () => {
    expect(searchIndustries('finance')).toContainEqual({
      slug: 'financial-services',
      label: 'Financial Services',
    });
    expect(searchIndustries('underwater basket weaving')).toEqual([]);
  });

  it('still offers a request for an exact miss when partial matches exist', () => {
    expect(lookupIndustry('bank')).toEqual({ type: 'miss' });
    expect(searchIndustries('bank')).toContainEqual({ slug: 'banking', label: 'Banking' });
    expect(isIndustryMiss('bank')).toBe(true);
    expect(isIndustryMiss('finance')).toBe(false);
    expect(isIndustryMiss('  ')).toBe(false);
  });

  it('preserves an industry query and selected tab for sign-in return', () => {
    const params = new URLSearchParams();
    seedDictionaryReturnParams(params, 'industry', '  New Industry  ');
    expect(params.get('industry')).toBe('  New Industry  ');
    expect(params.get('tab')).toBe('industry');
  });
});
