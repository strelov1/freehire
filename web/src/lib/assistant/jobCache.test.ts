import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/types';

const getJob = vi.fn<(slug: string) => Promise<Job>>();

vi.mock('$lib/api', () => ({
  api: { getJob: (slug: string) => getJob(slug) },
}));

const job = (slug: string): Job => ({ public_slug: slug }) as Job;

// jobCache is a plain module-level Map, so each test needs its own fresh instance
// rather than sharing state through the module cache vitest keeps between tests.
async function freshCache() {
  vi.resetModules();
  return import('./jobCache');
}

beforeEach(() => {
  getJob.mockReset();
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('loadJob', () => {
  it('dedupes concurrent calls for the same slug to one fetch', async () => {
    const { loadJob } = await freshCache();
    getJob.mockResolvedValue(job('go-dev-acme'));

    const [a, b] = await Promise.all([loadJob('go-dev-acme'), loadJob('go-dev-acme')]);

    expect(a).toEqual(b);
    expect(getJob).toHaveBeenCalledTimes(1);
  });

  it('evicts a rejected fetch so a later call retries', async () => {
    const { loadJob } = await freshCache();
    getJob.mockRejectedValueOnce(new Error('down'));
    await expect(loadJob('go-dev-acme')).rejects.toThrow('down');

    getJob.mockResolvedValueOnce(job('go-dev-acme'));
    await expect(loadJob('go-dev-acme')).resolves.toEqual(job('go-dev-acme'));
    expect(getJob).toHaveBeenCalledTimes(2);
  });

  // The cache used to hold every resolved entry for the rest of the session — only
  // a failed fetch was ever evicted. A long session (many search results, many job
  // decks) could name far more slugs than are worth keeping, so the cache now caps
  // its size and drops the least-recently-used entry rather than growing forever.
  it('caps the cache size, evicting the least-recently-used slug first', async () => {
    const { loadJob, MAX_ENTRIES } = await freshCache();
    getJob.mockImplementation((slug: string) => Promise.resolve(job(slug)));

    // Calling loadJob is what inserts into the cache (synchronously, before the
    // fetch resolves), so issuing all MAX_ENTRIES calls up front and awaiting them
    // together still inserts slug-0..slug-(N-1) in that order.
    await Promise.all(Array.from({ length: MAX_ENTRIES }, (_, i) => loadJob(`slug-${i}`)));
    expect(getJob).toHaveBeenCalledTimes(MAX_ENTRIES);

    // One more distinct slug pushes the cache past its cap.
    await loadJob('slug-overflow');
    getJob.mockClear();

    // The oldest entry (slug-0) was evicted, so it is fetched again...
    await loadJob('slug-0');
    expect(getJob).toHaveBeenCalledTimes(1);

    // ...while a slug that was still resident is served from the cache.
    getJob.mockClear();
    await loadJob('slug-overflow');
    expect(getJob).not.toHaveBeenCalled();
  });
});
