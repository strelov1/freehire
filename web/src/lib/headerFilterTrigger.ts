import type { ListSearchTarget } from './listSearch.svelte';

// Derives whether the header search box should host the All-filters trigger, and its
// badge count, from the active list-search target. Kept as a pure function (not inline
// in HeaderListSearch) so the visibility gate and badge count are unit-testable without
// a Svelte/rune runtime — the template just renders this. The trigger shows only where a
// page owns a filter modal (it published `openFilters`); the launcher and listless pages
// register no such target, so no trigger appears.

export interface HeaderFilterTrigger {
  visible: boolean;
  count: number;
}

export function headerFilterTrigger(target: ListSearchTarget | null): HeaderFilterTrigger {
  if (!target?.openFilters) return { visible: false, count: 0 };
  return { visible: true, count: target.activeFilters?.() ?? 0 };
}
