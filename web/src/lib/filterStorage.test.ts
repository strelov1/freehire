import { describe, it, expect, afterEach } from 'vitest';
import {
  loadJobFilters,
  saveJobFilters,
  hasChangedFilters,
  JOB_FILTERS_KEY,
  FILTERS_TOUCHED_COOKIE,
} from './filterStorage';

// A minimal in-memory localStorage stand-in for the Node test environment (where
// there is no browser storage). Individual tests swap in throwing/undefined
// variants to exercise the SSR / disabled-storage guards.
class MemoryStorage {
  #map = new Map<string, string>();
  getItem(k: string): string | null {
    return this.#map.has(k) ? (this.#map.get(k) as string) : null;
  }
  setItem(k: string, v: string): void {
    this.#map.set(k, v);
  }
  removeItem(k: string): void {
    this.#map.delete(k);
  }
}

describe('filterStorage', () => {
  afterEach(() => {
    // @ts-expect-error - clean up the globals we install per test
    delete globalThis.localStorage;
    // @ts-expect-error
    delete globalThis.document;
    // @ts-expect-error
    delete globalThis.location;
  });

  it('round-trips a non-empty query string through hire.jobFilters', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;

    saveJobFilters('regions=EU&seniority=senior');

    expect(store.getItem(JOB_FILTERS_KEY)).toBe('regions=EU&seniority=senior');
    expect(loadJobFilters()).toBe('regions=EU&seniority=senior');
  });

  it('removes the key when saving an empty string', () => {
    const store = new MemoryStorage();
    store.setItem(JOB_FILTERS_KEY, 'regions=EU');
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;

    saveJobFilters('');

    expect(store.getItem(JOB_FILTERS_KEY)).toBeNull();
    expect(loadJobFilters()).toBe('');
  });

  it('reports no filter history for a fresh browser, then history after any change', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;

    expect(hasChangedFilters()).toBe(false);

    saveJobFilters('regions=EU');
    expect(hasChangedFilters()).toBe(true);
  });

  it('keeps filter history set after a clear, so a cleared set stays distinct from a fresh visit', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;

    saveJobFilters('regions=EU');
    saveJobFilters(''); // clear

    expect(loadJobFilters()).toBe('');
    expect(hasChangedFilters()).toBe(true);
  });

  it('mirrors the touched marker into a server-readable cookie on any change', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;
    let written = '';
    // @ts-expect-error - minimal document stand-in capturing cookie writes
    globalThis.document = {
      set cookie(v: string) {
        written = v;
      },
    };
    // @ts-expect-error - https so the Secure attribute is emitted
    globalThis.location = { protocol: 'https:' };

    saveJobFilters('regions=EU');

    expect(written).toContain(`${FILTERS_TOUCHED_COOKIE}=1`);
    expect(written).toContain('path=/');
    expect(written).toContain('secure');
  });

  it('sets the touched cookie even when localStorage is unavailable (private mode)', () => {
    // No localStorage installed; the cookie is written before the storage guard.
    let written = '';
    // @ts-expect-error - minimal document stand-in
    globalThis.document = {
      set cookie(v: string) {
        written = v;
      },
    };

    saveJobFilters('regions=EU');

    expect(written).toContain(`${FILTERS_TOUCHED_COOKIE}=1`);
    // http (no location stub) omits Secure so the cookie is usable on localhost.
    expect(written).not.toContain('secure');
  });

  it('reports no filter history when storage is unavailable (SSR)', () => {
    // No globalThis.localStorage installed.
    expect(hasChangedFilters()).toBe(false);
  });

  it('returns empty and no-ops when storage is unavailable (SSR)', () => {
    // No globalThis.localStorage installed.
    expect(loadJobFilters()).toBe('');
    expect(() => saveJobFilters('regions=EU')).not.toThrow();
  });

  it('swallows storage access errors (private mode / quota)', () => {
    // @ts-expect-error - install a throwing stand-in
    globalThis.localStorage = {
      getItem() {
        throw new Error('denied');
      },
      setItem() {
        throw new Error('quota');
      },
      removeItem() {
        throw new Error('denied');
      },
    };

    expect(loadJobFilters()).toBe('');
    expect(() => saveJobFilters('regions=EU')).not.toThrow();
    expect(() => saveJobFilters('')).not.toThrow();
  });
});
