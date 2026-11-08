import { describe, expect, it } from 'vitest';
import { captureFirstTouch, gaClientId, gaSessionId, readFirstTouch } from './attribution';

function memory() {
  const map = new Map<string, string>();
  return {
    getItem: (k: string) => map.get(k) ?? null,
    setItem: (k: string, v: string) => void map.set(k, v),
  };
}
const never = () => false;

describe('captureFirstTouch', () => {
  it('keeps campaign tags, the referring host and the path — never the query string', () => {
    const store = memory();
    captureFirstTouch(
      {
        href: 'https://freehire.me/jobs?utm_source=linkedin&utm_medium=social&token=secret',
        referrer: 'https://www.linkedin.com/feed/?x=1',
      },
      store,
      'freehire.me',
      never,
    );
    expect(readFirstTouch(store)).toEqual({
      landing_path: '/jobs',
      utm_source: 'linkedin',
      utm_medium: 'social',
      referrer_host: 'www.linkedin.com',
    });
    expect(JSON.stringify(readFirstTouch(store))).not.toContain('secret');
  });

  it('is first-touch: the OAuth redirect back does not overwrite it', () => {
    const store = memory();
    captureFirstTouch({ href: 'https://freehire.me/?utm_source=ph', referrer: '' }, store, 'freehire.me', never);
    captureFirstTouch(
      { href: 'https://freehire.me/auth/callback', referrer: 'https://accounts.google.com/' },
      store,
      'freehire.me',
      (r) => new URL(r).hostname === 'accounts.google.com',
    );
    expect(readFirstTouch(store).utm_source).toBe('ph');
  });

  it('ignores the site itself and handoff hosts as a source', () => {
    const a = memory();
    captureFirstTouch({ href: 'https://freehire.me/', referrer: 'https://freehire.me/jobs' }, a, 'freehire.me', never);
    expect(readFirstTouch(a).referrer_host).toBeUndefined();
    const b = memory();
    captureFirstTouch(
      { href: 'https://freehire.me/', referrer: 'https://accounts.google.com/' },
      b,
      'freehire.me',
      () => true,
    );
    expect(readFirstTouch(b).referrer_host).toBeUndefined();
  });

  it('survives a storage that throws', () => {
    const broken = {
      getItem: () => {
        throw new Error('denied');
      },
      setItem: () => {
        throw new Error('denied');
      },
    };
    expect(() =>
      captureFirstTouch({ href: 'https://freehire.me/', referrer: '' }, broken, 'freehire.me', never),
    ).not.toThrow();
    expect(readFirstTouch(broken)).toEqual({});
  });
});

describe('GA cookie ids', () => {
  it('reads the client id and both session-id layouts', () => {
    const cookie = 'a=1; _ga=GA1.1.1234567890.1700000000; _ga_6P1PZ719T0=GS2.1.s1700001111$o1$g1';
    expect(gaClientId(cookie)).toBe('1234567890.1700000000');
    expect(gaSessionId(cookie, 'G-6P1PZ719T0')).toBe('1700001111');
    expect(gaSessionId('_ga_6P1PZ719T0=GS1.1.1700002222.3.0', 'G-6P1PZ719T0')).toBe('1700002222');
  });
  it('returns nothing without consent cookies', () => {
    expect(gaClientId('a=1')).toBeUndefined();
    expect(gaSessionId('a=1', 'G-6P1PZ719T0')).toBeUndefined();
  });
});
