// Pure summary of the header filter scope, driving the search-box trigger label.
// Kept out of the component so the roll-up logic (none / format / geo +N / both) is
// unit-testable without a DOM. A `ScopeSpec` names the facets a mode cares about, so
// the jobs feed (work format + regions/countries/cities) and the companies list
// (regions + remote_regions, no work format) share one summarizer.

import type { FacetStore } from './facets';
import { countryLabel } from './facets';
import { WORK_MODE_VALUES, type WorkMode } from './generated/contracts';
import { REGION_LABELS, WORK_MODE_LABELS } from './labels';

type ScopeIcon = 'globe' | WorkMode;

/** What the trigger draws: an overlapping icon cluster, then the head geography and
 *  its "+N" roll-up. `label` is the same summary in words — the icons carry meaning
 *  no screen reader can read, so it stays as the accessible name and the tooltip. */
export interface ScopeSummary {
  icons: ScopeIcon[];
  text: string;
  extra: number;
  label: string;
}

/** The region code that means "anywhere" — drawn as a globe rather than spelled out. */
const WORLDWIDE = 'global';

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

/** Derive what the trigger draws from the scope facets named by `spec`.
 *
 *  Two selections collapse into icons rather than words: a work format is already
 *  said by its glyph (a house is "remote"), and the worldwide region is a globe. So
 *  "Remote · Worldwide +3" draws as a house lapped by a globe, then "+3" — the words
 *  survive only in `label`, which the button wears as its accessible name and title.
 */
export function summarizeScope(
  store: Pick<FacetStore, 'facet'>,
  spec: ScopeSpec = JOBS_SCOPE,
): ScopeSummary {
  const modes = spec.format ? selected(store, spec.format) : [];
  const firstMode = WORK_MODE_VALUES.find((m) => modes.includes(m));

  const geo = spec.geo.flatMap((param) => selected(store, param).map((v) => ({ param, value: v })));
  const head = geo[0];
  const extra = Math.max(geo.length - 1, 0);
  // Only a region-shaped param carries the worldwide code; a city or a country
  // literally named "global" would still be a place, not everywhere.
  const headIsWorldwide =
    head !== undefined && head.param !== 'countries' && head.param !== 'cities' && head.value === WORLDWIDE;

  const icons: ScopeIcon[] = [];
  if (firstMode) icons.push(firstMode);
  if (headIsWorldwide) icons.push('globe');
  // Nothing iconic selected — the neutral globe is the resting state of the trigger.
  if (!icons.length) icons.push('globe');

  const text = head === undefined ? 'Location' : headIsWorldwide ? '' : geoLabel(head.param, head.value);

  const parts: string[] = [];
  if (firstMode) parts.push(workModeLabel(firstMode));
  if (head !== undefined) {
    const label = geoLabel(head.param, head.value);
    parts.push(extra > 0 ? `${label} +${extra}` : label);
  }

  return { icons, text, extra, label: parts.length ? parts.join(' · ') : 'Location' };
}
