import { describe, it, expect, vi } from 'vitest';
import { Paginator } from './paginated.svelte';

// The reactive Paginator needs the Svelte runtime, so it is exercised in the components
// project (*.spec.ts) rather than beside the pure helpers in paginated.svelte.test.ts.
describe('Paginator', () => {
  it('discards a slower earlier start that resolves after a newer one', async () => {
    let resolveOld!: (v: { items: string[]; hasMore: boolean }) => void;
    const fetch = vi
      .fn()
      .mockImplementationOnce(() => new Promise((r) => (resolveOld = r)))
      .mockImplementationOnce(() => Promise.resolve({ items: ['new'], hasMore: false }));
    const pager = new Paginator<string>(fetch);
    const old = pager.start();
    await pager.start();
    resolveOld({ items: ['old'], hasMore: true });
    await old;
    expect(pager.items).toEqual(['new']);
    expect(pager.hasMore).toBe(false);
  });

  it('discards a loadMore that resolves after a start replaced the list', async () => {
    let resolveMore!: (v: { items: string[]; hasMore: boolean }) => void;
    const fetch = vi
      .fn()
      .mockImplementationOnce(() => Promise.resolve({ items: ['a'], hasMore: true }))
      .mockImplementationOnce(() => new Promise((r) => (resolveMore = r)))
      .mockImplementationOnce(() => Promise.resolve({ items: ['x'], hasMore: false }));
    const pager = new Paginator<string>(fetch);
    await pager.start();
    const more = pager.loadMore();
    await pager.start();
    resolveMore({ items: ['b'], hasMore: false });
    await more;
    expect(pager.items).toEqual(['x']);
  });
});
