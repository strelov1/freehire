// Pure summary of the header Location & work-format scope, driving the search-box
// trigger label. Kept out of the component so the roll-up logic (none / format /
// geo +N / both) is unit-testable without a DOM. Reads the same facet params the
// popover writes: work_mode, regions, countries, cities.

import type { FacetStore } from './facets';
import { countryLabel } from './facets';
import { WORK_MODE_VALUES, type WorkMode } from './generated/contracts';
import { REGION_LABELS, WORK_MODE_LABELS } from './labels';

export type ScopeIcon = 'globe' | WorkMode;

const titleCase = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const workModeLabel = (code: string) => WORK_MODE_LABELS[code] ?? titleCase(code);

// A facet's full selection (include ∪ exclude): both narrow the scope, so both
// count toward the trigger summary.
function selected(store: Pick<FacetStore, 'facet'>, param: string): string[] {
  const f = store.facet(param);
  return [...f.include, ...f.exclude];
}

/** Derive the trigger's `{ icon, label }` from the current scope facets. */
export function summarizeScope(store: Pick<FacetStore, 'facet'>): { icon: ScopeIcon; label: string } {
  const modes = selected(store, 'work_mode');
  const firstMode = WORK_MODE_VALUES.find((m) => modes.includes(m));
  const icon: ScopeIcon = firstMode ?? 'globe';

  // Geography in display order: regions, then countries, then cities.
  const geo = [
    ...selected(store, 'regions').map((v) => REGION_LABELS[v] ?? v),
    ...selected(store, 'countries').map(countryLabel),
    ...selected(store, 'cities'),
  ];

  const parts: string[] = [];
  if (firstMode) parts.push(workModeLabel(firstMode));
  const head = geo[0];
  if (head !== undefined) {
    const extra = geo.length - 1;
    parts.push(extra > 0 ? `${head} +${extra}` : head);
  }

  return { icon, label: parts.length ? parts.join(' · ') : 'Location' };
}
