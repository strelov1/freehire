import { describe, expect, it, vi } from 'vitest';

import { GET } from './+server';

// The handler takes only what it reads, so the test builds only that. `setHeaders` is
// a spy rather than a stub because the cache rule is the point of the route as much as
// the payload is: a response a CDN may store replays one visitor's country to the next.
function call(headers: Record<string, string>) {
  const setHeaders = vi.fn();
  const response = (GET as unknown as (event: unknown) => Response)({
    request: new Request('https://freehire.me/geo/region', { headers }),
    setHeaders,
  });
  return { response, setHeaders };
}

describe('GET /geo/region', () => {
  it('returns the raw country beside the region', async () => {
    const { response } = call({ 'cf-ipcountry': 'BR' });
    await expect(response.json()).resolves.toMatchObject({ country: 'BR' });
  });

  it('is never stored, by a browser or a shared cache', () => {
    const { setHeaders } = call({ 'cf-ipcountry': 'BR' });
    expect(setHeaders).toHaveBeenCalledWith({ 'cache-control': 'private, no-store' });
  });

  it('tells a crawler nothing about where it is', async () => {
    const { response } = call({ 'cf-ipcountry': 'BR', 'user-agent': 'Googlebot/2.1' });
    await expect(response.json()).resolves.toEqual({ region: null, country: null });
  });

  it('reports a missing header as an unplaced visitor rather than inventing one', async () => {
    const { response } = call({});
    await expect(response.json()).resolves.toEqual({ region: null, country: null });
  });
});
