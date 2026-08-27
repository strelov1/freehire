import { describe, expect, it } from 'vitest';
import { matchSortEnabled } from './features';

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
