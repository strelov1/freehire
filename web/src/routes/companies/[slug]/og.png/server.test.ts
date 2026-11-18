import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api.js';

// Plain functions, not vi.fn(): a spy attaches its own handler to every promise it
// returns (to record settledResults), which would mark the rejection handled and hide
// exactly the bug this file pins.
const api: { getCompany: () => Promise<unknown>; searchJobs: () => Promise<unknown> } = {
  getCompany: () => Promise.reject(new Error('unset')),
  searchJobs: () => Promise.reject(new Error('unset')),
};

vi.mock('#lib/server/api.js', () => ({ serverApi: () => api }));
vi.mock('#lib/server/og/fonts.js', () => ({ loadOgFonts: async () => [] }));
vi.mock('#lib/server/og/logo.js', () => ({ resolveLogo: async () => null }));
vi.mock('#lib/server/og/render.js', () => ({ renderMarkupPng: async () => new Uint8Array() }));

const { GET } = await import('./+server.js');

type Event = Parameters<typeof GET>[0];

const unhandled: unknown[] = [];
const onUnhandled = (reason: unknown) => unhandled.push(reason);

afterEach(() => {
  process.off('unhandledRejection', onUnhandled);
  unhandled.length = 0;
});

describe('GET /companies/[slug]/og.png', () => {
  // In production both calls hit the same 10 s timeout when the API stalls; the count,
  // started first, fails first while the company fetch is still pending. An unhandled
  // rejection there killed the whole SSR cluster worker, not just this request.
  it('fails the request, not the process, when the job count fails before the company arrives', async () => {
    process.on('unhandledRejection', onUnhandled);
    api.searchJobs = () => Promise.reject(new ApiError(504, 'The API did not answer within 10000ms'));
    api.getCompany = () => new Promise((resolve) => setTimeout(() => resolve({ company: { name: 'Acme' } }), 20));

    await expect(GET({ params: { slug: 'acme' }, fetch } as unknown as Event)).rejects.toMatchObject({
      status: 504,
    });
    expect(unhandled).toEqual([]);
  });
});
