import { afterEach, expect, it, vi } from 'vitest';
import { createShortlistSession } from './api';

afterEach(() => vi.unstubAllGlobals());

it('reports the condition that prevented starting a search-bound conversation', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ error: 'Unsupported search condition: location' }, { status: 400 })));
  await expect(createShortlistSession('location=moon', 'f88efb02-b09f-4ec9-b1d7-8d7221bf7c6a')).rejects.toThrow('Unsupported search condition: location');
});
