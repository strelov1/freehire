import type { ListSearchTarget } from './listSearch.svelte';

// Derives whether the header search box should host the All-filters trigger, what it
// opens, and its badge count. Kept as a pure function (not inline in the component) so
// the gate, the precedence and the count are unit-testable without a Svelte/rune
// runtime — the template just renders this.

export interface HeaderFilterTrigger {
  /** What the trigger opens, or undefined when there is nothing to open — which is the
   *  same thing as "do not render it". One field rather than a `visible` flag beside
   *  it: two could disagree, and only one of them can be acted on. */
  open?: () => void;
  /** How many filters are applied behind the box. Only a list can have any. */
  count: number;
}

export function headerFilterTrigger(
  target: ListSearchTarget | null,
  /** A modal offered by whoever renders the box, for a page with no list to filter —
   *  the homepage, whose modal composes a search rather than narrowing one. */
  hostOpener?: () => void,
): HeaderFilterTrigger {
  // The list's own modal wins: it filters what is on screen, while the host's composes
  // a search for somewhere else. A page that has both would otherwise offer the wrong
  // one from a control that looks identical.
  if (target?.openFilters) return { open: target.openFilters, count: target.activeFilters?.() ?? 0 };
  // Nothing is applied yet on a listless page, so the badge stays off — a count of zero
  // and "no count" render the same, and there is no list behind it to have filtered.
  if (hostOpener) return { open: hostOpener, count: 0 };
  return { count: 0 };
}
