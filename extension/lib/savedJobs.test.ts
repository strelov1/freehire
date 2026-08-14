import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./freehire', () => ({ listSavedSlugs: vi.fn() }));

const { listSavedSlugs } = await import('./freehire');
const { ensureSavedLoaded, isSaved, markSaved, markUnsaved } = await import('./savedJobs');

describe('savedJobs', () => {
  beforeEach(() => {
    vi.mocked(listSavedSlugs).mockReset();
  });

  it('is empty before the set has loaded', () => {
    expect(isSaved('never-fetched')).toBe(false);
  });

  it('loads the saved set once and reports membership', async () => {
    vi.mocked(listSavedSlugs).mockResolvedValueOnce(['backend-engineer-acme']);
    await ensureSavedLoaded('token');
    expect(isSaved('backend-engineer-acme')).toBe(true);
    expect(isSaved('some-other-job')).toBe(false);
  });

  it('does not refetch on a second call', async () => {
    await ensureSavedLoaded('token');
    expect(listSavedSlugs).not.toHaveBeenCalled();
  });

  it('reflects a local save immediately', () => {
    markSaved('freshly-saved-job');
    expect(isSaved('freshly-saved-job')).toBe(true);
  });

  it('reflects a local unsave immediately', () => {
    markSaved('to-be-unsaved');
    markUnsaved('to-be-unsaved');
    expect(isSaved('to-be-unsaved')).toBe(false);
  });

  it('degrades to empty when the load fails, without throwing', async () => {
    vi.resetModules();
    vi.doMock('./freehire', () => ({
      listSavedSlugs: vi.fn().mockRejectedValue(new Error('network down')),
    }));
    const fresh = await import('./savedJobs');
    await expect(fresh.ensureSavedLoaded('token')).resolves.toBeUndefined();
    expect(fresh.isSaved('anything')).toBe(false);
  });
});
