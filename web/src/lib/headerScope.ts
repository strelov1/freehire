// Pure summary of the header filter scope, driving the search-box trigger label.
// Kept out of the component so the roll-up logic (none / format / geo +N / both) is
// unit-testable without a DOM. A `ScopeSpec` names the facets a mode cares about, so
// the jobs feed (work format + regions/countries/cities) and the companies list
// (regions + remote_regions, no work format) share one summarizer.

import type { FacetStore } from './facets';
import { countryLabel } from './facets';
import { WORK_MODE_VALUES, type WorkMode } from './generated/contracts';
import { REGION_LABELS, WORK_MODE_LABELS } from './labels';

export type ScopeIcon = 'globe' | WorkMode;

/** Which facets a mode summarizes: an optional work-format param (drives the icon)
 *  plus the ordered geography params rolled into "first +N". */
export interface ScopeSpec {
  format?: string;
  geo: string[];
}

export const JOBS_SCOPE: ScopeSpec = { format: 'work_mode', geo: ['regions', 'countries', 'cities'] };
export const COMPANIES_SCOPE: ScopeSpec = { geo: ['regions', 'remote_regions'] };

const titleCase = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const workModeLabel = (code: string) => WORK_MODE_LABELS[code] ?? titleCase(code);

// Per-param display label: countries via Intl, cities verbatim, every region-shaped
// param (regions/remote_regions) via the shared region vocabulary.
const geoLabel = (param: string, v: string) =>
  param === 'countries' ? countryLabel(v) : param === 'cities' ? v : (REGION_LABELS[v] ?? v);

// A facet's full selection (include ∪ exclude): both narrow the scope, so both
// count toward the trigger summary.
function selected(store: Pick<FacetStore, 'facet'>, param: string): string[] {
  const f = store.facet(param);
  return [...f.include, ...f.exclude];
}

/** Derive the trigger's `{ icon, label }` from the scope facets named by `spec`. */
export function summarizeScope(
  store: Pick<FacetStore, 'facet'>,
  spec: ScopeSpec = JOBS_SCOPE,
): { icon: ScopeIcon; label: string } {
  const modes = spec.format ? selected(store, spec.format) : [];
  const firstMode = WORK_MODE_VALUES.find((m) => modes.includes(m));
  const icon: ScopeIcon = firstMode ?? 'globe';

  const geo = spec.geo.flatMap((param) => selected(store, param).map((v) => geoLabel(param, v)));

  const parts: string[] = [];
  if (firstMode) parts.push(workModeLabel(firstMode));
  const head = geo[0];
  if (head !== undefined) {
    const extra = geo.length - 1;
    parts.push(extra > 0 ? `${head} +${extra}` : head);
  }

  return { icon, label: parts.length ? parts.join(' · ') : 'Location' };
}
