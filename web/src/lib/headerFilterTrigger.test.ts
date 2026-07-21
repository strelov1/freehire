import { describe, expect, it } from 'vitest';
import { headerFilterTrigger } from './headerFilterTrigger';
import type { ListSearchTarget } from './listSearch.svelte';

// A minimal target stub; only the fields the trigger reads matter. The rune-backed
// store is irrelevant here — the header consumes the plain `openFilters`/`activeFilters`
// callbacks, so a plain object exercises the contract without Svelte compilation.
function target(over: Partial<ListSearchTarget> = {}): ListSearchTarget {
  return { value: { q: '' }, setQuery: () => {}, ...over };
}

describe('headerFilterTrigger', () => {
  it('is hidden when there is no target (launcher/listless page)', () => {
    expect(headerFilterTrigger(null)).toEqual({ visible: false, count: 0 });
  });

  it('is hidden when the target owns no filter modal', () => {
    expect(headerFilterTrigger(target())).toEqual({ visible: false, count: 0 });
  });

  it('is visible with the active-filter count when the target exposes openFilters', () => {
    const t = target({ openFilters: () => {}, activeFilters: () => 2 });
    expect(headerFilterTrigger(t)).toEqual({ visible: true, count: 2 });
  });

  it('defaults the badge count to 0 when activeFilters is absent', () => {
    const t = target({ openFilters: () => {} });
    expect(headerFilterTrigger(t)).toEqual({ visible: true, count: 0 });
  });
});
