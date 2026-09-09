import { describe, it, expect } from 'vitest';
import { resolveGeoSource, eligibleForMeValues, isEligibleForMeOn } from './eligibleForMe';

// resolveGeoSource: the two-step precedence from specs/eligible-for-me-filter
// "The country/region source follows a fixed precedence and degrades gracefully".
describe('resolveGeoSource', () => {
  it('prefers the profile country over the edge-derived region', () => {
    expect(resolveGeoSource('id', 'apac')).toEqual({ kind: 'country', code: 'id' });
  });

  it('falls back to the edge-derived region when no profile country is set', () => {
    expect(resolveGeoSource(null, 'apac')).toEqual({ kind: 'region', code: 'apac' });
  });

  it('falls back to the edge-derived region when the profile country is empty', () => {
    expect(resolveGeoSource('', 'eu')).toEqual({ kind: 'region', code: 'eu' });
  });

  it('resolves to null when neither source is available', () => {
    expect(resolveGeoSource(null, null)).toBeNull();
  });

  it('lower-cases a profile country', () => {
    expect(resolveGeoSource('ID', null)).toEqual({ kind: 'country', code: 'id' });
  });
});

// eligibleForMeValues: what specs/eligible-for-me-filter "The filter composes
// existing location facet values" and "The safety values are always included" ask for.
describe('eligibleForMeValues', () => {
  it('stages the country plus the two safety regions for a country source', () => {
    expect(eligibleForMeValues({ kind: 'country', code: 'id' })).toEqual({
      countries: ['id'],
      regions: ['global', 'none'],
    });
  });

  it('stages the region plus the two safety regions for a region source', () => {
    expect(eligibleForMeValues({ kind: 'region', code: 'apac' })).toEqual({
      countries: [],
      regions: ['apac', 'global', 'none'],
    });
  });

  it('stages nothing for no source', () => {
    expect(eligibleForMeValues(null)).toEqual({ countries: [], regions: [] });
  });
});

// isEligibleForMeOn: the pill's on/off state, derived from what is actually staged
// rather than tracked separately — see design.md "Decisions".
describe('isEligibleForMeOn', () => {
  const countrySource = { kind: 'country' as const, code: 'id' };

  it('is on when every value the source would stage is already included', () => {
    expect(isEligibleForMeOn(countrySource, ['id'], ['global', 'none'])).toBe(true);
  });

  it('is on even when other, unrelated values are also included', () => {
    expect(isEligibleForMeOn(countrySource, ['id', 'us'], ['global', 'none', 'eu'])).toBe(true);
  });

  it('is off when one of the staged values is missing', () => {
    expect(isEligibleForMeOn(countrySource, ['id'], ['global'])).toBe(false);
  });

  it('is off when nothing is staged', () => {
    expect(isEligibleForMeOn(countrySource, [], [])).toBe(false);
  });

  it('is off when there is no source at all', () => {
    expect(isEligibleForMeOn(null, ['id'], ['global', 'none'])).toBe(false);
  });
});
