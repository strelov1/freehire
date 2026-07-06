import { describe, it, expect, afterEach } from 'vitest';
import { loadJobFilters, saveJobFilters, JOB_FILTERS_KEY } from './filterStorage';

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
