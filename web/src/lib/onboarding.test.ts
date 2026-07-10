import { describe, it, expect, afterEach } from 'vitest';
import { filtersFromParams } from './facetModel';
import {
  emptySelection,
  selectionsToQuery,
  narrowestFacet,
  bannerVisible,
  loadOnboardingState,
  markSeen,
  markDone,
  ONBOARDING_KEY,
  type OnboardingSelection,
} from './onboarding';

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

// Parse a selection's query back into filter facet state, so tests assert the
// selection survives the round-trip the live store performs (apply → URL → parse).
const facetsOf = (sel: OnboardingSelection) => filtersFromParams(new URLSearchParams(selectionsToQuery(sel))).facets;

describe('selectionsToQuery', () => {
  it('is empty for an empty selection', () => {
    expect(selectionsToQuery(emptySelection())).toBe('');
  });

  it('maps a role-only selection to the category facet', () => {
    const f = facetsOf({ ...emptySelection(), specializations: ['backend'] });
    expect(f.category?.include).toEqual(['backend']);
    expect(f.seniority?.include).toEqual([]);
    expect(f.skills?.include).toEqual([]);
  });

  it('maps several specializations to the category facet', () => {
    const f = facetsOf({ ...emptySelection(), specializations: ['backend', 'ai_engineering'] });
    expect(f.category?.include).toEqual(['backend', 'ai_engineering']);
  });

  it('maps several seniorities to the seniority facet (grade range)', () => {
    const f = facetsOf({ ...emptySelection(), seniorities: ['middle', 'senior'] });
    expect(f.seniority?.include).toEqual(['middle', 'senior']);
  });

  it('round-trips a role + stack selection', () => {
    const sel: OnboardingSelection = { ...emptySelection(), specializations: ['backend'], stack: ['Go', 'Kubernetes'] };
    const f = facetsOf(sel);
    expect(f.category?.include).toEqual(['backend']);
    expect(f.skills?.include).toEqual(['Go', 'Kubernetes']);
  });

  it('round-trips a full selection across all five facets', () => {
    const sel: OnboardingSelection = {
      specializations: ['frontend'],
      seniorities: ['senior'],
      workMode: 'remote',
      region: 'eu',
      stack: ['TypeScript'],
    };
    const f = facetsOf(sel);
    expect(f.category?.include).toEqual(['frontend']);
    expect(f.seniority?.include).toEqual(['senior']);
    expect(f.work_mode?.include).toEqual(['remote']);
    expect(f.regions?.include).toEqual(['eu']);
    expect(f.skills?.include).toEqual(['TypeScript']);
  });

  it('drops blank stack entries', () => {
    const sel: OnboardingSelection = { ...emptySelection(), stack: ['Go', '  ', ''] };
    expect(facetsOf(sel).skills?.include).toEqual(['Go']);
  });
});

describe('narrowestFacet', () => {
  it('returns null when no relaxable facet is set', () => {
    const f = filtersFromParams(new URLSearchParams(selectionsToQuery({ ...emptySelection(), specializations: ['backend'] })));
    // only category (never relaxed) is set
    expect(narrowestFacet(f)).toBeNull();
  });

  it('peels stack first, then region, then seniority — never the role', () => {
    const full = filtersFromParams(
      new URLSearchParams(
        selectionsToQuery({ specializations: ['backend'], seniorities: ['senior'], workMode: undefined, region: 'eu', stack: ['Go'] }),
      ),
    );
    expect(narrowestFacet(full)).toBe('skills');

    const noStack = filtersFromParams(
      new URLSearchParams(selectionsToQuery({ ...emptySelection(), specializations: ['backend'], seniorities: ['senior'], region: 'eu' })),
    );
    expect(narrowestFacet(noStack)).toBe('regions');

    const seniorityOnly = filtersFromParams(
      new URLSearchParams(selectionsToQuery({ ...emptySelection(), specializations: ['backend'], seniorities: ['senior'] })),
    );
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

  it('defaults to unseen and records seen / done', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;

    expect(loadOnboardingState()).toBe('unseen');
    markSeen();
    expect(store.getItem(ONBOARDING_KEY)).toBe('seen');
    expect(loadOnboardingState()).toBe('seen');
    markDone();
    expect(loadOnboardingState()).toBe('done');
  });

  it('does not downgrade a completed onboarding back to seen', () => {
    const store = new MemoryStorage();
    // @ts-expect-error - install the stand-in
    globalThis.localStorage = store;

    markDone();
    markSeen();
    expect(loadOnboardingState()).toBe('done');
  });

  it('no-ops safely when storage is unavailable or throws', () => {
    // No storage installed (SSR).
    expect(loadOnboardingState()).toBe('unseen');
    expect(() => markSeen()).not.toThrow();
    expect(() => markDone()).not.toThrow();

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
    expect(() => markDone()).not.toThrow();
  });
});
