import { describe, it, expect, afterEach } from 'vitest';
import { ApiError } from './api';
import type { SavedSearch } from './types';
import {
  alertName,
  matchedSavedSearch,
  ensureSaved,
  setPendingAlert,
  consumePendingAlert,
  PENDING_ALERT_KEY,
  type SavedSearchesPort,
} from './saveSearchAlert';

// In-memory localStorage stand-in for the Node test env (mirrors filterStorage.test.ts).
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

const search = (id: number, query: string, name = `s${id}`): SavedSearch =>
  ({ id, name, query, public_slug: '', author_label: '', created_at: '', updated_at: '' }) as SavedSearch;

// A SavedSearchesPort double that records create calls and grows its list on success.
function makeSaved(opts: {
  items?: SavedSearch[];
  createImpl?: (name: string, query: string) => Promise<SavedSearch>;
}): { port: SavedSearchesPort; createCalls: Array<{ name: string; query: string }> } {
  const items = [...(opts.items ?? [])];
  const createCalls: Array<{ name: string; query: string }> = [];
  return {
    createCalls,
    port: {
      ensureLoaded: async () => {},
      get items() {
        return items;
      },
      create: async (name, query) => {
        createCalls.push({ name, query });
        if (opts.createImpl) return opts.createImpl(name, query);
        const row = search(items.length + 100, query, name);
        items.push(row);
        return row;
      },
    },
  };
}

describe('alertName', () => {
  it('falls back to a stable default for an empty query', () => {
    expect(alertName('')).toBe('Job alert');
  });
  it('uses facet labels for a facet query', () => {
    const name = alertName('category=backend&seniority=senior');
    expect(name).toContain('Senior');
    expect(name).toContain('Backend');
  });
  it('includes the text query and stays within 100 chars', () => {
    const name = alertName('q=react');
    expect(name).toContain('react');
    expect(name.length).toBeLessThanOrEqual(100);
  });
});

describe('matchedSavedSearch', () => {
  it('matches by canonical query regardless of param order', () => {
    const items = [search(1, 'seniority=senior&category=backend')];
    expect(matchedSavedSearch('category=backend&seniority=senior', items)?.id).toBe(1);
  });
  it('returns undefined when nothing matches', () => {
    expect(matchedSavedSearch('category=backend', [search(1, 'category=frontend')])).toBeUndefined();
  });
});

describe('ensureSaved', () => {
  it('creates an auto-named saved search when the query is new', async () => {
    const { port, createCalls } = makeSaved({ items: [] });
    const set = await ensureSaved('category=backend', port);
    expect(createCalls).toEqual([{ name: alertName('category=backend'), query: 'category=backend' }]);
    expect(set.query).toBe('category=backend');
  });

  it('reuses an existing saved search matching the query (no duplicate)', async () => {
    const { port, createCalls } = makeSaved({ items: [search(7, 'category=backend')] });
    const set = await ensureSaved('category=backend', port);
    expect(set.id).toBe(7);
    expect(createCalls).toHaveLength(0);
  });

  it('reuses a concurrently-created same-query set on a 409', async () => {
    const items = [search(7, 'category=backend')];
    const port: SavedSearchesPort = {
      ensureLoaded: async () => {},
      get items() {
        return items;
      },
      // Pretend another tab already created it just before us: matchedSavedSearch finds
      // id 7, so create is never reached — but if a 409 races in, the raced re-check wins.
      create: async () => {
        throw new ApiError(409, 'duplicate name');
      },
    };
    const set = await ensureSaved('category=backend', port);
    expect(set.id).toBe(7);
  });

  it('retries with a suffixed name on a 409 name collision for a new query', async () => {
    let first = true;
    const { port, createCalls } = makeSaved({
      items: [],
      createImpl: async (name, query) => {
        if (first) {
          first = false;
          throw new ApiError(409, 'duplicate name');
        }
        return search(9, query, name);
      },
    });
    const set = await ensureSaved('category=backend', port);
    expect(set.id).toBe(9);
    expect(createCalls).toHaveLength(2);
    expect(createCalls[1]!.name).not.toBe(createCalls[0]!.name);
  });
});

describe('pendingAlert', () => {
  afterEach(() => {
    // @ts-expect-error clean up the global
    delete globalThis.localStorage;
  });

  it('sets, consumes once, then clears', () => {
    const store = new MemoryStorage();
    // @ts-expect-error install stand-in
    globalThis.localStorage = store;
    setPendingAlert('category=backend');
    expect(store.getItem(PENDING_ALERT_KEY)).toBe('category=backend');
    expect(consumePendingAlert()).toBe('category=backend');
    expect(consumePendingAlert()).toBeNull();
  });

  it('round-trips an empty-query (all jobs) save', () => {
    // @ts-expect-error install stand-in
    globalThis.localStorage = new MemoryStorage();
    setPendingAlert('');
    expect(consumePendingAlert()).toBe('');
  });

  it('no-ops safely when storage is unavailable', () => {
    expect(() => setPendingAlert('x')).not.toThrow();
    expect(consumePendingAlert()).toBeNull();
  });
});
