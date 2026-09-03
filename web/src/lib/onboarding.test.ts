import { describe, it, expect, afterEach } from 'vitest';
import { emptyFilters, type JobFilters } from './facetModel';
import { bannerVisible, loadOnboardingState, markSeen, narrowestFacet, ONBOARDING_KEY } from './onboarding';

// Reuse the same in-memory localStorage stand-in shape as filterStorage.test.ts:
// the Node test env has no browser storage, so tests install a global per case.
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

// Builds a JobFilters fixture with just the facets narrowestFacet cares about set,
// bypassing the standard filter UI's own query-string round-trip (not under test here).
function filtersWith(include: Partial<Record<'skills' | 'regions' | 'seniority' | 'category', string[]>>): JobFilters {
  const f = emptyFilters();
  for (const [param, values] of Object.entries(include)) {
    f.facets[param] = { include: values, exclude: [], matchAll: false };
  }
  return f;
}

describe('narrowestFacet', () => {
  it('returns null when no relaxable facet is set', () => {
    // only category (never relaxed) is set
    expect(narrowestFacet(filtersWith({ category: ['backend'] }))).toBeNull();
  });

  it('peels stack first, then region, then seniority — never the role', () => {
    const full = filtersWith({ category: ['backend'], seniority: ['senior'], regions: ['eu'], skills: ['Go'] });
    expect(narrowestFacet(full)).toBe('skills');

    const noStack = filtersWith({ category: ['backend'], seniority: ['senior'], regions: ['eu'] });
    expect(narrowestFacet(noStack)).toBe('regions');

    const seniorityOnly = filtersWith({ category: ['backend'], seniority: ['senior'] });
    expect(narrowestFacet(seniorityOnly)).toBe('seniority');
  });
});

describe('bannerVisible', () => {
  it('shows only for an unseen visitor with no active filters', () => {
    expect(bannerVisible('unseen', false)).toBe(true);
    expect(bannerVisible('unseen', true)).toBe(false);
    expect(bannerVisible('seen', false)).toBe(false);
    expect(bannerVisible('done', false)).toBe(false);
  });
});

describe('onboarding lifecycle state', () => {
  afterEach(() => {
    // @ts-expect-error - clean up the global we install per test
    delete globalThis.localStorage;
  });

  it('defaults to unseen and records seen', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;

    expect(loadOnboardingState()).toBe('unseen');
    markSeen();
    expect(store.getItem(ONBOARDING_KEY)).toBe('seen');
    expect(loadOnboardingState()).toBe('seen');
  });

  // 'done' is a legacy value: the now-retired wizard used to write it on completion,
  // and nothing writes it anymore — but a browser that stored it before this change
  // must still be read correctly (and markSeen() must not downgrade it).
  it('reads a legacy done value and does not downgrade it back to seen', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;
    store.setItem(ONBOARDING_KEY, 'done');

    expect(loadOnboardingState()).toBe('done');
    markSeen();
    expect(loadOnboardingState()).toBe('done');
  });

  it('no-ops safely when storage is unavailable or throws', () => {
    // No storage installed (SSR).
    expect(loadOnboardingState()).toBe('unseen');
    expect(() => markSeen()).not.toThrow();

    // @ts-expect-error - throwing stand-in (private mode / quota)
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
    expect(loadOnboardingState()).toBe('unseen');
    expect(() => markSeen()).not.toThrow();
  });
});
