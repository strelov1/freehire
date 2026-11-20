import { afterEach, describe, expect, it, vi } from 'vitest';
vi.mock('wxt/browser', () => ({ browser: {} }));
import { matchPage } from './pageMatch';

describe('matchPage lookup URL', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('drops credentials and fragment but keeps the query', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      text: async () => JSON.stringify({ data: null, meta: { ats_board: true } }),
    }));
    vi.stubGlobal('fetch', fetchMock);

    await matchPage('https://u:p@jobs.example.com/acme/123?for=acme&token=9#sso=SECRET', 't', () => false);

    const [requested] = fetchMock.mock.calls[0] as unknown as [string];
    const sent = new URL(requested).searchParams.get('url');
    expect(new URL(requested).pathname).toBe('/api/v1/jobs/find');
    expect(sent).toBe('https://jobs.example.com/acme/123?for=acme&token=9');
  });
});
