import { describe, it, expect, vi, beforeEach } from 'vitest';

const getJob = vi.fn();
vi.mock('../freehire', () => ({ getJob: (slug: string, token: string) => getJob(slug, token) }));
vi.mock('../auth', () => ({ getToken: async () => 'tok', HIRE_ORIGIN: 'http://localhost:5173' }));

const { loadJob, resetJobCache } = await import('./jobCache');

beforeEach(() => {
  getJob.mockReset();
  resetJobCache();
});

describe('loadJob', () => {
  // The chat reassigns its state on every streamed token, so a card that fetched
  // per render would issue a request per token.
  it('fetches a slug once however many cards ask for it', async () => {
    getJob.mockResolvedValue({ public_slug: 'go-dev', title: 'Go Dev', company: 'Acme', location: '' });

    await Promise.all([loadJob('go-dev'), loadJob('go-dev'), loadJob('go-dev')]);

    expect(getJob).toHaveBeenCalledTimes(1);
  });

  it('fetches each distinct slug', async () => {
    getJob.mockResolvedValue({ public_slug: 'x', title: 'X', company: '', location: '' });

    await Promise.all([loadJob('a'), loadJob('b')]);

    expect(getJob).toHaveBeenCalledTimes(2);
  });

  // A cached rejection would strand the card on its fallback for the whole
  // session, however many times the user reopens the panel.
  it('forgets a failed fetch so a later render can retry', async () => {
    getJob.mockRejectedValueOnce(new Error('offline'));
    await expect(loadJob('go-dev')).rejects.toThrow('offline');

    getJob.mockResolvedValue({ public_slug: 'go-dev', title: 'Go Dev', company: '', location: '' });
    await expect(loadJob('go-dev')).resolves.toMatchObject({ title: 'Go Dev' });

    expect(getJob).toHaveBeenCalledTimes(2);
  });

  it('reaches hire with the stored session token', async () => {
    getJob.mockResolvedValue({ public_slug: 'go-dev', title: 'Go Dev', company: '', location: '' });

    await loadJob('go-dev');

    expect(getJob).toHaveBeenCalledWith('go-dev', 'tok');
  });
});
