import { afterEach, expect, it, vi } from 'vitest';
import { createSession } from './api';

afterEach(() => vi.unstubAllGlobals());

it('reports the server reason when starting a conversation fails', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ error: 'Plan limit reached' }, { status: 400 })));
  await expect(createSession()).rejects.toThrow('Plan limit reached');
});

it('starts a support conversation on the support routes, with no preset query', async () => {
  const fetchMock = vi.fn().mockResolvedValue(Response.json({ data: { id: 's1' } }));
  vi.stubGlobal('fetch', fetchMock);
  await createSession('support');
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/support/sessions');
});
