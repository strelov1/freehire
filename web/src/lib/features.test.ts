import { describe, expect, it } from 'vitest';
import { matchSortEnabled, openWithinEnabled } from './features';

// The match sort ships dark: the backend accepts ?sort=match from day one, but the
// vectors it ranks against only exist after a full index rebuild. Until then the sort
// returns a near-empty feed, so the control stays hidden and the flag is what reveals
// it — no redeploy, just an env change and a restart.
describe('matchSortEnabled', () => {
  it('is off when the flag is absent', () => {
    expect(matchSortEnabled({})).toBe(false);
  });

  it('is off for an empty value', () => {
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: '' })).toBe(false);
  });

  it('is on for the documented value', () => {
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: '1' })).toBe(true);
  });

  it('accepts the other truthy spellings an ops edit is likely to use', () => {
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: 'true' })).toBe(true);
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: 'TRUE' })).toBe(true);
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: 'on' })).toBe(true);
  });

  // A flag that reads "0" or "false" as ON is how a feature ships by accident.
  it('is off for the falsy spellings', () => {
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: '0' })).toBe(false);
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: 'false' })).toBe(false);
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: 'off' })).toBe(false);
  });

  it('is off for anything unrecognized, rather than guessing', () => {
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: 'yes please' })).toBe(false);
  });

  it('tolerates surrounding whitespace, which a .env edit leaves behind', () => {
    expect(matchSortEnabled({ PUBLIC_MATCH_SORT: ' 1 ' })).toBe(true);
  });
});

// The open-within bound ships dark for the same shape of reason as the match sort: the
// API honours ?open_within_days from the moment the binary rolls out, but it filters on
// a document field that only exists once a full rebuild has written it. Until then the
// bound matches almost nothing — a thin feed, not an error, so nothing alerts.
describe('openWithinEnabled', () => {
  it('is off when the flag is absent', () => {
    expect(openWithinEnabled({})).toBe(false);
  });

  it('is off for an empty value', () => {
    expect(openWithinEnabled({ PUBLIC_OPEN_WITHIN: '' })).toBe(false);
  });

  it('is on for the documented value', () => {
    expect(openWithinEnabled({ PUBLIC_OPEN_WITHIN: '1' })).toBe(true);
  });

  it('accepts the other truthy spellings an ops edit is likely to use', () => {
    expect(openWithinEnabled({ PUBLIC_OPEN_WITHIN: 'true' })).toBe(true);
    expect(openWithinEnabled({ PUBLIC_OPEN_WITHIN: 'TRUE' })).toBe(true);
    expect(openWithinEnabled({ PUBLIC_OPEN_WITHIN: 'on' })).toBe(true);
  });

  it('is off for the falsy spellings and for anything unrecognized', () => {
    expect(openWithinEnabled({ PUBLIC_OPEN_WITHIN: '0' })).toBe(false);
    expect(openWithinEnabled({ PUBLIC_OPEN_WITHIN: 'false' })).toBe(false);
    expect(openWithinEnabled({ PUBLIC_OPEN_WITHIN: 'yes please' })).toBe(false);
  });

  it('does not read the other flag', () => {
    expect(openWithinEnabled({ PUBLIC_MATCH_SORT: '1' })).toBe(false);
    expect(matchSortEnabled({ PUBLIC_OPEN_WITHIN: '1' })).toBe(false);
  });
});
