import { describe, expect, it } from 'vitest';
import { headerFilterTrigger } from './headerFilterTrigger';
import type { ListSearchTarget } from './listSearch.svelte';

// A minimal target stub; only the fields the trigger reads matter. The rune-backed
// store is irrelevant here — the header consumes the plain `openFilters`/`activeFilters`
// callbacks, so a plain object exercises the contract without Svelte compilation.
function target(over: Partial<ListSearchTarget> = {}): ListSearchTarget {
  return { value: { q: '' }, commitQuery: () => {}, ...over };
}

describe('headerFilterTrigger', () => {
  it('has nothing to open with no target and no host modal', () => {
    expect(headerFilterTrigger(null)).toEqual({ count: 0 });
  });

  it('has nothing to open when the target owns no filter modal', () => {
    expect(headerFilterTrigger(target())).toEqual({ count: 0 });
  });

  it("opens the list's modal, with its active-filter count", () => {
    const openFilters = () => {};
    const t = target({ openFilters, activeFilters: () => 2 });
    expect(headerFilterTrigger(t)).toEqual({ open: openFilters, count: 2 });
  });

  it('defaults the badge count to 0 when activeFilters is absent', () => {
    const t = target({ openFilters: () => {} });
    expect(headerFilterTrigger(t).count).toBe(0);
  });

  it("opens the host's modal on a page with no list, with no badge", () => {
    const hostOpener = () => {};
    expect(headerFilterTrigger(null, hostOpener)).toEqual({ open: hostOpener, count: 0 });
  });

  it("prefers the list's modal over the host's", () => {
    // Both filter, but only one filters what is on screen. The control looks identical
    // either way, so the wrong choice here is invisible until someone picks a facet and
    // the list under it does not move.
    const openFilters = () => {};
    const hostOpener = () => {};
    const t = target({ openFilters, activeFilters: () => 3 });
    expect(headerFilterTrigger(t, hostOpener)).toEqual({ open: openFilters, count: 3 });
  });
});
