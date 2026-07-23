import { describe, it, expect, vi } from 'vitest';
import { loadWithRetry } from './paginated.svelte';
import { ApiError } from './api';

// The reactive Paginator can't be instantiated here — its `$state` fields need a
// Svelte runtime this test env doesn't provide — so we cover the retry decision at
// its pure core: `loadWithRetry` (which `Paginator.start` wraps 1:1).

describe('loadWithRetry', () => {
  it('returns the value on first success, no retry', async () => {
    const fetch = vi.fn(async () => 42);
    await expect(loadWithRetry(fetch)).resolves.toBe(42);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('rethrows a server ApiError immediately, without retrying', async () => {
    const err = new ApiError(500, 'boom');
    const fetch = vi.fn(async () => {
      throw err;
    });
    await expect(loadWithRetry(fetch)).rejects.toBe(err);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('retries a cancelled fetch and self-heals', async () => {
    vi.useFakeTimers();
    const fetch = vi
      .fn<() => Promise<number>>()
      .mockRejectedValueOnce(new TypeError('Load failed'))
      .mockResolvedValueOnce(7);
    const done = loadWithRetry(fetch);
    await vi.runAllTimersAsync();
    await expect(done).resolves.toBe(7);
    expect(fetch).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });

  it('gives up after exhausting retries on a persistent network failure', async () => {
    vi.useFakeTimers();
    const err = new TypeError('Failed to fetch');
    const fetch = vi.fn(async () => {
      throw err;
    });
    const done = loadWithRetry(fetch);
    const assertion = expect(done).rejects.toBe(err);
    await vi.runAllTimersAsync();
    await assertion;
    expect(fetch).toHaveBeenCalledTimes(3); // initial + 2 retries
    vi.useRealTimers();
  });
});
