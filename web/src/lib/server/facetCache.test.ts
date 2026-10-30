import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { FacetCounts } from '#lib/types.js';
import { createFacetCache } from './facetCache.js';

const result = (total: number): FacetCounts => ({ total, facets: {}, stats: {} });

describe('createFacetCache', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('answers a repeated request from memory', async () => {
    const live = vi.fn().mockResolvedValue(result(7));
    const cached = createFacetCache()(live);

    await cached(new URLSearchParams({ category: 'backend' }), { facets: ['skills'] });
    const second = await cached(new URLSearchParams({ category: 'backend' }), { facets: ['skills'] });

    expect(second.total).toBe(7);
    expect(live).toHaveBeenCalledTimes(1);
  });

  it('shares one in-flight request between concurrent callers', async () => {
    const live = vi.fn().mockResolvedValue(result(1));
    const cached = createFacetCache()(live);

    await Promise.all([cached(new URLSearchParams()), cached(new URLSearchParams())]);

    expect(live).toHaveBeenCalledTimes(1);
  });

  it('keys on the filter and the requested facets', async () => {
    const live = vi.fn().mockResolvedValue(result(1));
    const cached = createFacetCache()(live);

    await cached(new URLSearchParams({ category: 'backend' }));
    await cached(new URLSearchParams({ category: 'frontend' }));
    await cached(new URLSearchParams({ category: 'backend' }), { facets: ['skills'] });

    expect(live).toHaveBeenCalledTimes(3);
  });

  it('asks again once the entry has expired', async () => {
    const live = vi.fn().mockResolvedValue(result(1));
    const cached = createFacetCache()(live);

    await cached(new URLSearchParams());
    vi.advanceTimersByTime(10 * 60_000 + 1);
    await cached(new URLSearchParams());

    expect(live).toHaveBeenCalledTimes(2);
  });

  it('does not keep a failure', async () => {
    const live = vi.fn().mockRejectedValueOnce(new Error('down')).mockResolvedValue(result(3));
    const cached = createFacetCache()(live);

    await expect(cached(new URLSearchParams())).rejects.toThrow('down');
    expect((await cached(new URLSearchParams())).total).toBe(3);
  });

  it('never caches the disjunctive mode', async () => {
    const live = vi.fn().mockResolvedValue(result(1));
    const cached = createFacetCache()(live);

    await cached(new URLSearchParams(), { disjunctive: true });
    await cached(new URLSearchParams(), { disjunctive: true });

    expect(live).toHaveBeenCalledTimes(2);
  });

  it('never caches the disjunctive mode carried in the params', async () => {
    const live = vi.fn().mockResolvedValue(result(1));
    const cached = createFacetCache()(live);

    await cached(new URLSearchParams({ disjunctive: '1' }));
    await cached(new URLSearchParams({ disjunctive: '1' }));

    expect(live).toHaveBeenCalledTimes(2);
  });

  it('shares entries between renders that bring their own counter', async () => {
    const withCache = createFacetCache();
    const first = vi.fn().mockResolvedValue(result(1));
    const second = vi.fn().mockResolvedValue(result(2));

    await withCache(first)(new URLSearchParams());
    const answer = await withCache(second)(new URLSearchParams());

    expect(answer.total).toBe(1);
    expect(second).not.toHaveBeenCalled();
  });

  it('evicts the oldest entry past its capacity', async () => {
    const live = vi.fn().mockResolvedValue(result(1));
    const cached = createFacetCache(2)(live);

    await cached(new URLSearchParams({ q: 'a' }));
    await cached(new URLSearchParams({ q: 'b' }));
    await cached(new URLSearchParams({ q: 'c' }));
    await cached(new URLSearchParams({ q: 'a' }));

    expect(live).toHaveBeenCalledTimes(4);
  });
});
