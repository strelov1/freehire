// The pure job-filter model: the facet/filter types, their URL <-> state
// (de)serialization, and the per-value sign transitions. No SvelteKit or Svelte
// runes here, so this module is unit-testable in plain Node and importable from
// both the reactive store (filters.ts) and the staged-edit surface. Param names
// match what the search API (GET /api/v1/jobs/search) expects, including the
// `<param>_exclude` and `<param>_mode=and` conventions.

import { FACETS, type FacetSelection } from './facets';
import type { UserProfile } from './types';

/** The three states a facet value can hold. */
export type Sign = 'off' | 'include' | 'exclude';

/** One facet's selection: the included values and the excluded values (a value is
 *  in at most one set), plus whether the *included* values are ANDed (match all)
 *  instead of ORed (match any). Excluded values are always ANDed — a job matches
 *  only if it has none of them. Include and exclude coexist in one facet, so a
 *  user can include some values and exclude others at the same time.
 *
 *  Structurally identical to (and aliased from) facets.ts's `FacetSelection`, the
 *  shape `FacetSection` reads — one canonical type so the two can't drift. */
export type FacetState = FacetSelection;

/** The fields the browse list can be ordered by. Only `posted_at` (the source's
 *  posting date, newest-first) is offered today; the type stays a union so a
 *  future sort (e.g. salary) re-introduces an option without reshaping callers. */
export type SortField = 'posted_at';

/** Default (and currently only) browse order: freshest by posting date. Kept out
 *  of the URL (see filtersToParams) so the default reads as a clean, sort-less URL
 *  and the backend's own empty-query default stays the single source of truth. */
export const DEFAULT_SORT: SortField = 'posted_at';

export interface JobFilters {
  q: string;
  /** Facet state keyed by the facet's query param (see FACETS). */
  facets: Record<string, FacetState>;
  visa: boolean;
  salaryMin: number | null;
  /** Freshness: keep only jobs posted within the last N days (null = any age).
   *  Serialized as `posted_within_days`; the backend turns it into a posted_ts
   *  range filter relative to request time. */
  postedWithinDays: number | null;
  sort: SortField;
}

export function emptyFacet(): FacetState {
  return { include: [], exclude: [], matchAll: false };
}

function emptyFacets(): Record<string, FacetState> {
  const out: Record<string, FacetState> = {};
  for (const f of FACETS) out[f.param] = emptyFacet();
  return out;
}

export function emptyFilters(): JobFilters {
  return { q: '', facets: emptyFacets(), visa: false, salaryMin: null, postedWithinDays: null, sort: DEFAULT_SORT };
}

// ---- URL serialization ----

/** Serialize filters to URL query params (the shape the search API reads). */
export function filtersToParams(f: JobFilters): URLSearchParams {
  const p = new URLSearchParams();
  if (f.q) p.set('q', f.q);
  for (const def of FACETS) {
    const st = f.facets[def.param];
    if (!st) continue;
    for (const v of st.include) p.append(def.param, v);
    for (const v of st.exclude) p.append(`${def.param}_exclude`, v);
    // AND-mode is per facet and only meaningful with more than one included value.
    if (st.matchAll && st.include.length > 1) p.set(`${def.param}_mode`, 'and');
  }
  if (f.visa) p.set('visa_sponsorship', 'true');
  if (f.salaryMin != null) p.set('salary_min', String(f.salaryMin));
  if (f.postedWithinDays != null) p.set('posted_within_days', String(f.postedWithinDays));
  // Omit the default sort: a clean URL leans on the backend's empty-query default.
  if (f.sort !== DEFAULT_SORT) p.set('sort', f.sort);
  return p;
}

/** Parse filters back from URL query params. Include and exclude are independent
 *  sets; if a value appears in both (a malformed or legacy link), exclude wins and
 *  it is dropped from include so a value carries exactly one sign. */
export function filtersFromParams(p: URLSearchParams): JobFilters {
  const f = emptyFilters();
  f.q = p.get('q') ?? '';
  for (const def of FACETS) {
    // URL params aren't guaranteed unique (shared/edited links, crawlers), but a
    // facet's values are a set — the store's transitions enforce that on user
    // input, so the URL parse must too. A repeated value otherwise reaches a chip
    // list keyed by value, and Svelte throws `each_key_duplicate` on hydration.
    const exclude = [...new Set(p.getAll(`${def.param}_exclude`))];
    const excludeSet = new Set(exclude);
    const include = [...new Set(p.getAll(def.param))].filter((v) => !excludeSet.has(v));
    const matchAll = p.get(`${def.param}_mode`) === 'and';
    f.facets[def.param] = { include, exclude, matchAll };
  }
  f.visa = p.get('visa_sponsorship') === 'true';
  const salary = Number(p.get('salary_min'));
  f.salaryMin = p.get('salary_min') && !Number.isNaN(salary) ? salary : null;
  // Freshness is a positive whole number of days; anything else (absent, zero,
  // negative, non-numeric) reads as "any age", matching the backend's own guard.
  const days = Number(p.get('posted_within_days'));
  f.postedWithinDays = Number.isInteger(days) && days > 0 ? days : null;
  // Sort isn't user-selectable today, so it's never read from the URL — it stays
  // the default seeded by emptyFilters().
  return f;
}

/** Total selected facet values (plus visa/salary/freshness) — drives the mobile badge. */
export function activeFilterCount(f: JobFilters): number {
  let n = 0;
  for (const def of FACETS) {
    const st = f.facets[def.param];
    if (st) n += st.include.length + st.exclude.length;
  }
  if (f.visa) n += 1;
  if (f.salaryMin != null) n += 1;
  if (f.postedWithinDays != null) n += 1;
  return n;
}

/** Normalize a search query string to its canonical form (parse → re-serialize),
 *  so two filter sets that differ only in param order or stale/unknown params
 *  compare equal. Used to detect which saved search matches the current filters. */
export function canonicalQuery(query: string): string {
  return filtersToParams(filtersFromParams(new URLSearchParams(query))).toString();
}

// ---- per-value sign transitions (pure: FacetState -> FacetState) ----

/** Which set a value currently belongs to. */
export function signOf(st: FacetState, v: string): Sign {
  if (st.include.includes(v)) return 'include';
  if (st.exclude.includes(v)) return 'exclude';
  return 'off';
}

/** Force a value into the given state, removing it from the other set first. */
export function facetSetSign(st: FacetState, v: string, sign: Sign): FacetState {
  const include = st.include.filter((x) => x !== v);
  const exclude = st.exclude.filter((x) => x !== v);
  if (sign === 'include') include.push(v);
  else if (sign === 'exclude') exclude.push(v);
  return { ...st, include, exclude };
}

/** Pills interaction: off → include → exclude → off. */
export function facetCycle(st: FacetState, v: string): FacetState {
  const s = signOf(st, v);
  return facetSetSign(st, v, s === 'off' ? 'include' : s === 'include' ? 'exclude' : 'off');
}

/** Select-dropdown interaction: pick adds to include; picking a selected value
 *  (in either set) clears it. */
export function facetPick(st: FacetState, v: string): FacetState {
  return facetSetSign(st, v, signOf(st, v) === 'off' ? 'include' : 'off');
}

/** Per-chip toggle: flip a value between include and exclude. */
export function facetToggleSign(st: FacetState, v: string): FacetState {
  return facetSetSign(st, v, signOf(st, v) === 'include' ? 'exclude' : 'include');
}

/** Token-input add: put a value into include; no-op on blank or a value already
 *  present in either set. */
export function facetAdd(st: FacetState, raw: string): FacetState {
  const v = raw.trim();
  if (!v || signOf(st, v) !== 'off') return st;
  return facetSetSign(st, v, 'include');
}

/** Remove a value from the facet entirely (both sets). */
export function facetRemove(st: FacetState, v: string): FacetState {
  return facetSetSign(st, v, 'off');
}

/** Build a fresh filter set seeded from a user profile — the reset-and-seed behind
 *  "Apply my profile". Specializations become `category` values and skills become `skills`.
 *  The optional location block flattens into the location facets: work_modes → `work_mode`;
 *  regions from the remote reach ∪ relocation targets; countries from the remote reach ∪
 *  base ∪ relocation targets; cities from the base ∪ relocation targets; and `relocation`
 *  staged as supported+required when the user is open to relocating. The flatten is lossy
 *  (base vs relocation merge) — the filter is a convenience narrowing of "places relevant to
 *  me". Trimming/dedup come free from facetAdd, so unions of overlapping lists are safe. */
export function filtersFromProfile(profile: UserProfile): JobFilters {
  const seed = (values: string[]) => values.reduce(facetAdd, emptyFacet());
  const f = emptyFilters();
  f.facets.category = seed(profile.specializations);
  f.facets.skills = seed(profile.skills);

  const loc = profile.location_preferences;
  if (loc) {
    // Relocation targets only count when the user is actually open to relocating — `open`
    // gates the whole relocation contribution (targets and the relocation facet alike).
    const reloc = loc.relocation.open ? loc.relocation : { regions: [], countries: [], cities: [] };
    f.facets.work_mode = seed(loc.work_modes ?? []);
    f.facets.regions = seed([...(loc.remote.regions ?? []), ...(reloc.regions ?? [])]);
    f.facets.countries = seed([
      ...(loc.remote.countries ?? []),
      ...(loc.base.country ? [loc.base.country] : []),
      ...(reloc.countries ?? []),
    ]);
    f.facets.cities = seed([
      ...(loc.base.city ? [loc.base.city] : []),
      ...(reloc.cities ?? []),
    ]);
    if (loc.relocation.open) f.facets.relocation = seed(['supported', 'required']);
  }
  return f;
}
