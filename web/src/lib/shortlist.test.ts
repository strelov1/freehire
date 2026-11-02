import { afterEach, describe, expect, it, vi } from 'vitest';
import { ShortlistSession, shortlistQuery } from './shortlist';

const create = vi.fn(async (query: string, requestId: string) => ({
  id: requestId, preset: 'chat', label: '', search_context: { query, brief: '' },
}));
const get = vi.fn();

afterEach(() => { vi.unstubAllGlobals(); create.mockClear(); get.mockReset(); });

function storage() {
  const values = new Map<string, string>();
  vi.stubGlobal('sessionStorage', {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => values.set(key, value),
    removeItem: (key: string) => values.delete(key),
  });
}

describe('shortlist search handoff', () => {
  it('captures constraints without pagination and does not mutate the page query', () => {
    const params = new URLSearchParams('q=backend&salary_min=80000&skills=-php,go&hide_seen=true&page=4&limit=20&offset=60');
    expect(shortlistQuery(params)).toBe('hide_seen=true&q=backend&salary_min=80000&skills=-php%2Cgo');
    expect(params.get('page')).toBe('4');
  });

  it('creates only on activation, reuses an in-flight creation, and keeps the captured search', async () => {
    storage();
    const owner = new ShortlistSession(7);
    expect(create).not.toHaveBeenCalled();
    const first = owner.open('q=go', create, get);
    const same = owner.open('q=rust', create, get);
    expect(first).toBe(same);
    const result = await same;
    expect(result.search_context?.query).toBe('q=go');
    expect(create).toHaveBeenCalledTimes(1);
    expect(await owner.open('q=rust', create, get)).toBe(result);
    expect(create).toHaveBeenCalledTimes(1);
  });

  it('retries failed creation after reload with the original request and search', async () => {
    storage();
    const failure = vi.fn(async () => { throw new Error('offline'); });
    await expect(new ShortlistSession(7).open('q=go', failure, get)).rejects.toThrow('offline');
    const request = failure.mock.calls[0];
    await new ShortlistSession(7).open('q=rust', create, get);
    expect(create.mock.calls[0]).toEqual(request);
  });

  it('restores the owned conversation and does not create when it is missing', async () => {
    storage();
    const original = await new ShortlistSession(7).open('q=go', create, get);
    get.mockRejectedValue(new Error('session not found'));
    await expect(new ShortlistSession(7).open('q=rust', create, get)).rejects.toThrow('session not found');
    expect(get).toHaveBeenCalledWith(original.id);
    expect(create).toHaveBeenCalledTimes(1);
  });

  it('isolates account storage and explicitly starts a new search', async () => {
    storage();
    const firstOwner = new ShortlistSession(7);
    const original = await firstOwner.open('q=go', create, get);
    const other = await new ShortlistSession(8).open('q=rust', create, get);
    expect(other.search_context?.query).toBe('q=rust');
    expect(other.id).not.toBe(original.id);
    firstOwner.startNew();
    const next = await firstOwner.open('q=python', create, get);
    expect(next.search_context?.query).toBe('q=python');
    expect(next.id).not.toBe(original.id);
  });

  it('works with disabled browser storage', async () => {
    vi.stubGlobal('sessionStorage', { getItem() { throw new Error('disabled'); }, setItem() { throw new Error('disabled'); } });
    const result = await new ShortlistSession(7).open('', create, get);
    expect(result.search_context?.query).toBe('');
  });
});
