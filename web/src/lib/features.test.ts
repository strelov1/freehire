import { describe, expect, it } from 'vitest';
import { openWithinEnabled } from './features';

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

  // Reads its OWN variable and nothing else. Worth pinning even now that it is the last
  // flag here: a retired name left set on a host must not turn anything on.
  it('reads only its own variable', () => {
    expect(openWithinEnabled({ PUBLIC_MATCH_SORT: '1' })).toBe(false);
    expect(openWithinEnabled({ SOMETHING_ELSE: '1' })).toBe(false);
  });
});
