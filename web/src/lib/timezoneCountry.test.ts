import { describe, expect, it } from 'vitest';

import { countryOfTimezone } from './timezoneCountry';

// The table is generated from tzdata, so these tests are not checking arithmetic — they
// pin the cases where a guess from the zone's NAME would have been wrong, and the ones
// that actually appear in freehire's data.

describe('countryOfTimezone', () => {
  it('resolves the zones that carry most of the data', () => {
    // Asia/Calcutta is the single most common value upstream — 168 people — and it is a
    // renamed zone that no longer appears in tzdata's main table. Getting it from the
    // backward links is the difference between the largest group having a flag or not.
    expect(countryOfTimezone('Asia/Calcutta')).toBe('in');
    expect(countryOfTimezone('America/New_York')).toBe('us');
    expect(countryOfTimezone('Europe/Zurich')).toBe('ch');
    expect(countryOfTimezone('America/Sao_Paulo')).toBe('br');
  });

  it('resolves zones whose name does not name their country', () => {
    // The reason this table is generated rather than inferred: nothing in these strings
    // says which country they belong to.
    expect(countryOfTimezone('Europe/Kiev')).toBe('ua');
    expect(countryOfTimezone('Atlantic/Reykjavik')).toBe('is');
    expect(countryOfTimezone('Pacific/Honolulu')).toBe('us');
  });

  it('returns undefined for a zone that names an offset rather than a place', () => {
    // 'Etc/GMT-3' and 'UTC' both occur in the data. There is no flag for an offset, and
    // inventing one would be worse than showing none.
    expect(countryOfTimezone('Etc/GMT-3')).toBeUndefined();
    expect(countryOfTimezone('UTC')).toBeUndefined();
  });

  it('returns undefined rather than throwing on missing or unknown input', () => {
    expect(countryOfTimezone(undefined)).toBeUndefined();
    expect(countryOfTimezone('')).toBeUndefined();
    expect(countryOfTimezone('Not/AZone')).toBeUndefined();
  });
});
