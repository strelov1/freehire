import { describe, expect, it, vi } from 'vitest';
import { ensureTechMarks, techMarks } from './techmarksLoad.svelte';

describe('tech marks loader', () => {
  it('resolves a known skill from the split sheet', async () => {
    ensureTechMarks();
    await vi.waitFor(() => expect(techMarks()?.python?.path).toEqual(expect.any(String)));
  });
});
