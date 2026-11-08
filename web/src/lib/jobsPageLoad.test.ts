import { describe, expect, it, vi } from 'vitest';
import { ApiError, createApi } from '#lib/api.js';

vi.mock('#lib/server/api.js', () => ({
  serverApi: (fetchImpl: typeof fetch) => createApi(fetchImpl),
}));

const { load } = await import('../routes/jobs/+page.server');

function event(query: string, fetchImpl: typeof fetch): Parameters<typeof load>[0] {
  const url = new URL('https://freehire.test/jobs');
  url.searchParams.set('q', query);
  return { url, fetch: fetchImpl, request: new Request(url) } as Parameters<typeof load>[0];
}

describe('jobs page search validation', () => {
  it('returns a SvelteKit 400 with the search validation message', async () => {
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockResolvedValue(Response.json({ error: 'q has more than 5 OR alternatives' }, { status: 400 }));

    await expect(load(event('go OR rust OR java OR python OR ruby OR kotlin', fetchImpl))).rejects.toMatchObject({
      status: 400,
      body: { message: 'q has more than 5 OR alternatives' },
    });
    expect(fetchImpl).toHaveBeenCalledOnce();
  });

  it('keeps genuine upstream faults unexpected', async () => {
    const fetchImpl = vi
      .fn<typeof fetch>()
      .mockResolvedValue(Response.json({ error: 'internal server error' }, { status: 500 }));

    await expect(load(event('go', fetchImpl))).rejects.toBeInstanceOf(ApiError);
  });

  it('still renders valid search results with their filters', async () => {
    const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(Response.json({ data: [], meta: { total: 0 } }));

    await expect(load(event('go OR rust', fetchImpl))).resolves.toMatchObject({
      initial: { items: [], total: 0 },
      filterParams: 'q=go+OR+rust',
      pageNumber: 1,
    });
  });
});
