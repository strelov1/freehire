import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// The loader memoises at module level, so each test takes a fresh copy of the module.
async function freshLoader() {
  vi.resetModules();
  return (await import('./geoCities')).loadCityTable;
}

const TABLE = { br: { 'sao paulo': [-23.56, -46.63] } };

describe('loadCityTable', () => {
  const fetchMock = vi.fn<typeof fetch>();
  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal('fetch', fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('fetches the static table once, however many callers ask', async () => {
    fetchMock.mockResolvedValue(new Response(JSON.stringify(TABLE)));
    const load = await freshLoader();
    const [a, b] = await Promise.all([load(), load()]);
    expect(await load()).toEqual(TABLE);
    expect(a).toEqual(TABLE);
    expect(b).toBe(a);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith('/geo-cities.json');
  });

  it('answers null on a network failure and does not ask again', async () => {
    fetchMock.mockRejectedValue(new TypeError('offline'));
    const load = await freshLoader();
    expect(await load()).toBeNull();
    expect(await load()).toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('answers null on an error status rather than parsing the error page', async () => {
    fetchMock.mockResolvedValue(new Response('<html>not found</html>', { status: 404 }));
    const load = await freshLoader();
    expect(await load()).toBeNull();
  });

  it('answers null on a body that is not JSON', async () => {
    fetchMock.mockResolvedValue(new Response('<html>'));
    const load = await freshLoader();
    expect(await load()).toBeNull();
  });
});
