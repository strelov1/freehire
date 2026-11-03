import { afterEach, expect, it, vi } from 'vitest';
import { createSession } from './api';

afterEach(() => vi.unstubAllGlobals());

it('reports the server reason when starting a conversation fails', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ error: 'Plan limit reached' }, { status: 400 })));
  await expect(createSession()).rejects.toThrow('Plan limit reached');
});
