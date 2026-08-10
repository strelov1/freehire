import { describe, it, expect } from 'vitest';
import { parseAuthRedirect } from './auth';

const base = 'https://abcdefghijklmnop.chromiumapp.org/';

describe('parseAuthRedirect', () => {
  it('returns the token when state matches', () => {
    const url = `${base}#state=s1&token=fhk_secret`;
    expect(parseAuthRedirect(url, 's1')).toBe('fhk_secret');
  });

  it('throws when state does not match (anti-forgery)', () => {
    const url = `${base}#state=other&token=fhk_secret`;
    expect(() => parseAuthRedirect(url, 's1')).toThrow();
  });

  it('surfaces an error indication instead of a token', () => {
    const url = `${base}#error=access_denied&state=s1`;
    expect(() => parseAuthRedirect(url, 's1')).toThrow(/access_denied/);
  });

  it('throws when there is no token', () => {
    const url = `${base}#state=s1`;
    expect(() => parseAuthRedirect(url, 's1')).toThrow();
  });
});
