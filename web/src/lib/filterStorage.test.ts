import { describe, it, expect, afterEach } from 'vitest';
import {
  geoScopeOffered,
  JOB_FILTERS_KEY,
  loadJobFilters,
  markGeoScopeOffered,
  saveJobFilters,
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
    // @ts-expect-error - clean up the global we install per test
    delete globalThis.localStorage;
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

describe('geo-scope marker', () => {
  afterEach(() => {
    // @ts-expect-error - clean up the global we install per test
    delete globalThis.localStorage;
  });

  it('is unset in a browser that has never been offered the guess', () => {
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = new MemoryStorage();

    expect(geoScopeOffered()).toBe(false);
  });

  it('stays set once marked', () => {
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = new MemoryStorage();

    markGeoScopeOffered();

    expect(geoScopeOffered()).toBe(true);
  });

  // The whole reason the marker is its own key: clearing the filters removes
  // hire.jobFilters, so a guess keyed on "storage is empty" would come back on the
  // next visit and undo the clear, every time.
  it('survives clearing the filter set', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;

    markGeoScopeOffered();
    saveJobFilters('');

    expect(loadJobFilters()).toBe('');
    expect(geoScopeOffered()).toBe(true);
  });

  it('reports a successful write', () => {
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = new MemoryStorage();

    expect(markGeoScopeOffered()).toBe(true);
  });

  it('reports "offered" when storage is unavailable (SSR)', () => {
    // No globalThis.localStorage installed.
    expect(geoScopeOffered()).toBe(true);
    expect(markGeoScopeOffered()).toBe(false);
  });

  // Reads and writes fail independently: a quota-exhausted store answers getItem
  // and throws on setItem. Shrugging that off would offer the guess again on every
  // visit — an offer nobody can dismiss — so the caller has to hear about it.
  it('reports a failed write even when reads work', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install a stand-in that reads but cannot write
    globalThis.localStorage = {
      getItem: (k: string) => store.getItem(k),
      setItem() {
        throw new Error('quota');
      },
      removeItem: (k: string) => store.removeItem(k),
    };

    expect(markGeoScopeOffered()).toBe(false);
    expect(geoScopeOffered()).toBe(false);
  });

  // An unrecordable guess is one that would re-apply on every single load and fight
  // anyone who cleared it, so the safe reading of an unreadable marker is "done".
  it('reports "offered" when storage throws (private mode / quota)', () => {
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

    expect(geoScopeOffered()).toBe(true);
    expect(markGeoScopeOffered()).toBe(false);
  });
});
