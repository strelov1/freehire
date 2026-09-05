import { describe, it, expect, afterEach } from 'vitest';
import { ApiError } from './api';
import type { SavedSearch, Subscription } from './types';
import {
  alertName,
  matchedSavedSearch,
  ensureSaved,
  ensureSubscribed,
  setPendingAlert,
  consumePendingAlert,
  PENDING_ALERT_KEY,
  type SavedSearchesPort,
  type SubscriptionsPort,
} from './saveSearchAlert';
import { must } from './utils';

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
  ({ id, name, query, created_at: '', updated_at: '' }) as SavedSearch;

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

  it('reuses a concurrently-created same-query set discovered only after the 409', async () => {
    // Empty at call time, so the pre-create check (matchedSavedSearch before create())
    // finds nothing and ensureSaved proceeds to create() — unlike a items-seeded setup,
    // which would satisfy the pre-create check and never reach create() or the catch
    // block at all.
    const items: SavedSearch[] = [];
    const port: SavedSearchesPort = {
      ensureLoaded: async () => {},
      get items() {
        return items;
      },
      // Simulate another tab's concurrent create landing between our pre-create check
      // and our own create() call: by the time our create() rejects with 409, the
      // racing item is already visible in `items`, so it's the catch block's raced
      // re-check — not the pre-create check — that must find it.
      create: async () => {
        items.push(search(7, 'category=backend'));
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
    expect(must(createCalls[1]).name).not.toBe(must(createCalls[0]).name);
  });
});

const sub = (id: number, savedSearchId: number, channel: string): Subscription =>
  ({ id, saved_search_id: savedSearchId, channel, active: true }) as Subscription;

// A SubscriptionsPort double. `held` is what the caller currently has; `server` is what
// a refresh() would read back. Seeding them differently is how a stale list is staged.
function makeSubs(opts: {
  held?: Subscription[];
  server?: Subscription[];
  addImpl?: () => Promise<void>;
}): {
  port: SubscriptionsPort;
  held: Subscription[];
  calls: { ensureLoaded: number; add: number; refresh: number };
} {
  let held = [...(opts.held ?? [])];
  const calls = { ensureLoaded: 0, add: 0, refresh: 0 };
  return {
    calls,
    get held() {
      return held;
    },
    port: {
      ensureLoaded: async () => {
        calls.ensureLoaded++;
      },
      find: (id, ch) => held.find((s) => s.saved_search_id === id && s.channel === ch),
      add: async (id, ch) => {
        calls.add++;
        if (opts.addImpl) return opts.addImpl();
        held = [sub(held.length + 100, id, ch), ...held];
      },
      refresh: async () => {
        calls.refresh++;
        held = [...(opts.server ?? [])];
      },
    },
  };
}

describe('ensureSubscribed', () => {
  it('creates the subscription when the channel is not yet subscribed', async () => {
    const s = makeSubs({ held: [] });
    await ensureSubscribed(5, 'email', s.port);
    expect(s.calls.add).toBe(1);
    expect(s.calls.refresh).toBe(0);
    expect(s.held).toHaveLength(1);
  });

  it('does not re-POST a channel it already holds', async () => {
    const s = makeSubs({ held: [sub(1, 5, 'email')] });
    await ensureSubscribed(5, 'email', s.port);
    expect(s.calls.add).toBe(0);
  });

  it('subscribes per channel, not per saved search', async () => {
    const s = makeSubs({ held: [sub(1, 5, 'email')] });
    await ensureSubscribed(5, 'telegram', s.port);
    expect(s.calls.add).toBe(1);
  });

  // The reported bug: the list was stale, so the pre-check found nothing, the POST
  // conflicted, and the caller was left still not holding the row — drawing the channel
  // "off" over a live subscription, so the next tap conflicted again. Treating the
  // conflict as success means ending up holding it, not merely not throwing.
  it('adopts the server row on a 409 rather than surfacing it', async () => {
    const s = makeSubs({
      held: [],
      server: [sub(42, 5, 'email')],
      addImpl: async () => {
        throw new ApiError(409, 'already subscribed to this saved search on this channel');
      },
    });
    await ensureSubscribed(5, 'email', s.port);
    expect(s.calls.refresh).toBe(1);
    expect(s.port.find(5, 'email')?.id).toBe(42);
  });

  it('rethrows anything that is not a 409', async () => {
    const s = makeSubs({
      held: [],
      addImpl: async () => {
        throw new ApiError(500, 'boom');
      },
    });
    await expect(ensureSubscribed(5, 'email', s.port)).rejects.toThrow('boom');
    expect(s.calls.refresh).toBe(0);
  });

  // A write racing an in-flight load is discarded by the snapshot that lands after it,
  // so the load has to settle before the pre-check reads the list.
  it('waits for the load before deciding whether to POST', async () => {
    const s = makeSubs({ held: [sub(1, 5, 'email')] });
    await ensureSubscribed(5, 'email', s.port);
    expect(s.calls.ensureLoaded).toBe(1);
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
