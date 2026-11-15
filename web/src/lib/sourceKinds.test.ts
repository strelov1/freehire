import { beforeEach, expect, it, vi } from 'vitest';

const listSources = vi.hoisted(() => vi.fn());
vi.mock('./api', () => ({ api: { listSources } }));
beforeEach(() => {
  vi.resetModules();
  listSources.mockReset();
});

it('shares one catalogue request across concurrently mounted cards and navigation', async () => {
  listSources.mockResolvedValue([{ source: 'whatjobs', kind: 'aggregator' }]);
  const { loadSourceKinds } = await import('./sourceKinds');
  const [first, second] = await Promise.all([loadSourceKinds(), loadSourceKinds()]);
  expect(first.get('whatjobs')).toBe('aggregator');
  expect(second).toBe(first);
  expect(await loadSourceKinds()).toBe(first);
  expect(listSources).toHaveBeenCalledOnce();
});

it('allows a later mount to retry when the catalogue was unavailable', async () => {
  listSources
    .mockRejectedValueOnce(new Error('Offline'))
    .mockResolvedValueOnce([{ source: 'greenhouse', kind: 'ats' }]);
  const { loadSourceKinds } = await import('./sourceKinds');
  expect((await loadSourceKinds()).size).toBe(0);
  expect((await loadSourceKinds()).get('greenhouse')).toBe('ats');
});
