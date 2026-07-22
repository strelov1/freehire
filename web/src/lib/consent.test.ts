import { afterEach, describe, expect, it, vi } from 'vitest';
import { CONSENT_KEY, needsBanner, readConsent, regionNeedsConsent, writeConsent } from './consent';

// A minimal in-memory localStorage stub for the read/write round-trip tests. The
// pure functions must degrade to a no-op / null when storage is unavailable (SSR),
// which the "no storage" cases below assert.
function stubLocalStorage(): Storage {
  const map = new Map<string, string>();
  const storage = {
    getItem: (k: string) => (map.has(k) ? map.get(k)! : null),
    setItem: (k: string, v: string) => void map.set(k, v),
    removeItem: (k: string) => void map.delete(k),
    clear: () => map.clear(),
    key: (i: number) => [...map.keys()][i] ?? null,
    get length() {
      return map.size;
    },
  } as Storage;
  vi.stubGlobal('localStorage', storage);
  return storage;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

// Pure region classification from a browser timezone. No runes, no localStorage —
// exercised in the plain-node vitest environment, per the frontend convention.

describe('regionNeedsConsent', () => {
  it('treats European timezones as consent-required', () => {
    expect(regionNeedsConsent('Europe/Berlin')).toBe(true);
    expect(regionNeedsConsent('Europe/London')).toBe(true);
    expect(regionNeedsConsent('Europe/Paris')).toBe(true);
  });

  it('treats EEA Atlantic zones as consent-required', () => {
    expect(regionNeedsConsent('Atlantic/Canary')).toBe(true);
    expect(regionNeedsConsent('Atlantic/Madeira')).toBe(true);
    expect(regionNeedsConsent('Atlantic/Reykjavik')).toBe(true);
  });

  it('treats non-European timezones as not consent-required', () => {
    expect(regionNeedsConsent('America/New_York')).toBe(false);
    expect(regionNeedsConsent('Asia/Tokyo')).toBe(false);
    expect(regionNeedsConsent('Australia/Sydney')).toBe(false);
    // A US Atlantic-coast-sounding zone must not be misread as an EEA Atlantic one.
    expect(regionNeedsConsent('America/Halifax')).toBe(false);
  });

  it('fails toward asking when the timezone is missing or unknown', () => {
    expect(regionNeedsConsent('')).toBe(true);
    expect(regionNeedsConsent('Not/AZone')).toBe(true);
  });
});

describe('needsBanner', () => {
  it('is true only for a consent-required visitor who has made no choice', () => {
    expect(needsBanner(true, null)).toBe(true);
  });

  it('is false once a choice exists, regardless of region', () => {
    expect(needsBanner(true, 'granted')).toBe(false);
    expect(needsBanner(true, 'denied')).toBe(false);
  });

  it('is false for a visitor who is not consent-required', () => {
    expect(needsBanner(false, null)).toBe(false);
    expect(needsBanner(false, 'granted')).toBe(false);
  });
});

describe('readConsent / writeConsent', () => {
  it('round-trips a granted choice', () => {
    stubLocalStorage();
    writeConsent('granted');
    expect(readConsent()).toBe('granted');
    expect(localStorage.getItem(CONSENT_KEY)).toBe('granted');
  });

  it('round-trips a denied choice', () => {
    stubLocalStorage();
    writeConsent('denied');
    expect(readConsent()).toBe('denied');
  });

  it('returns null when no choice has been stored', () => {
    stubLocalStorage();
    expect(readConsent()).toBe(null);
  });

  it('treats an unrecognized stored value as no choice', () => {
    const storage = stubLocalStorage();
    storage.setItem(CONSENT_KEY, 'garbage');
    expect(readConsent()).toBe(null);
  });

  it('degrades to null and never throws when storage is unavailable', () => {
    vi.stubGlobal('localStorage', undefined);
    expect(() => readConsent()).not.toThrow();
    expect(readConsent()).toBe(null);
    expect(() => writeConsent('granted')).not.toThrow();
  });
});
