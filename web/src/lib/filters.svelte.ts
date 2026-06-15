// Job search filters: the model, its URL <-> state (de)serialization, and a
// reactive store that mirrors the filters into the URL query so they survive
// reloads, sharing, and back/forward. Param names match what the search API
// (GET /api/v1/jobs/search) expects, including the `<param>_exclude` and
// `<param>_mode=and` conventions.

import { page } from '$app/state';
import { replaceState } from '$app/navigation';
import { FACETS } from './facets';

/** One facet's selection: the chosen values, whether it filters by inclusion or
 *  exclusion, and (for facets that allow it) whether selected values are ANDed
 *  (match all) instead of ORed (match any). */
export interface FacetState {
  values: string[];
  exclude: boolean;
  matchAll: boolean;
}

/** The fields the browse list can be ordered by (both newest-first). `posted_at`
 *  is the source's posting date; `created_at` is when we ingested the job. */
export type SortField = 'posted_at' | 'created_at';

/** Default browse order: freshest by posting date. Kept out of the URL (see
 *  filtersToParams) so the default reads as a clean, sort-less URL and the
 *  backend's own empty-query default stays the single source of truth. */
export const DEFAULT_SORT: SortField = 'posted_at';

export interface JobFilters {
  q: string;
  /** Facet state keyed by the facet's query param (see FACETS). */
  facets: Record<string, FacetState>;
  visa: boolean;
  salaryMin: number | null;
  sort: SortField;
}

function emptyFacet(): FacetState {
  return { values: [], exclude: false, matchAll: false };
}

function emptyFacets(): Record<string, FacetState> {
  const out: Record<string, FacetState> = {};
  for (const f of FACETS) out[f.param] = emptyFacet();
  return out;
}

export function emptyFilters(): JobFilters {
  return { q: '', facets: emptyFacets(), visa: false, salaryMin: null, sort: DEFAULT_SORT };
}

/** Serialize filters to URL query params (the shape the search API reads). */
export function filtersToParams(f: JobFilters): URLSearchParams {
  const p = new URLSearchParams();
  if (f.q) p.set('q', f.q);
  for (const def of FACETS) {
    const st = f.facets[def.param];
    if (!st || st.values.length === 0) continue;
    const key = st.exclude ? `${def.param}_exclude` : def.param;
    for (const v of st.values) p.append(key, v);
    // AND-mode is per facet and only meaningful with more than one included value.
    if (st.matchAll && !st.exclude && st.values.length > 1) {
      p.set(`${def.param}_mode`, 'and');
    }
  }
  if (f.visa) p.set('visa_sponsorship', 'true');
  if (f.salaryMin != null) p.set('salary_min', String(f.salaryMin));
  // Omit the default sort: a clean URL leans on the backend's empty-query default.
  if (f.sort !== DEFAULT_SORT) p.set('sort', f.sort);
  return p;
}

/** Parse filters back from URL query params. Exclude takes precedence over
 *  include when both appear for the same facet. */
export function filtersFromParams(p: URLSearchParams): JobFilters {
  const f = emptyFilters();
  f.q = p.get('q') ?? '';
  for (const def of FACETS) {
    const exclude = p.getAll(`${def.param}_exclude`);
    const include = p.getAll(def.param);
    const matchAll = p.get(`${def.param}_mode`) === 'and';
    if (exclude.length > 0) f.facets[def.param] = { values: exclude, exclude: true, matchAll };
    else if (include.length > 0) f.facets[def.param] = { values: include, exclude: false, matchAll };
  }
  f.visa = p.get('visa_sponsorship') === 'true';
  const salary = Number(p.get('salary_min'));
  f.salaryMin = p.get('salary_min') && !Number.isNaN(salary) ? salary : null;
  // Only `created_at` is a non-default sort; anything else falls back to default.
  f.sort = p.get('sort') === 'created_at' ? 'created_at' : DEFAULT_SORT;
  return f;
}

/** Total selected facet values (plus visa/salary) — drives the mobile badge. */
export function activeFilterCount(f: JobFilters): number {
  let n = 0;
  for (const def of FACETS) n += f.facets[def.param]?.values.length ?? 0;
  if (f.visa) n += 1;
  if (f.salaryMin != null) n += 1;
  return n;
}

/** Normalize a search query string to its canonical form (parse → re-serialize),
 *  so two filter sets that differ only in param order or stale/unknown params
 *  compare equal. Used to detect which saved search matches the current filters. */
export function canonicalQuery(query: string): string {
  return filtersToParams(filtersFromParams(new URLSearchParams(query))).toString();
}

/** Reactive filter state mirrored into the URL. Owned by the jobs view; all
 *  mutations go through its methods so every change updates state and URL. */
export class FilterStore {
  value = $state<JobFilters>(emptyFilters());

  /** Seed from the current URL params (passed by the view from `page.url`), so
   *  the same filters render on the server and hydrate on the client. */
  constructor(initial?: URLSearchParams) {
    this.value = filtersFromParams(initial ?? new URLSearchParams());
  }

  get active(): number {
    return activeFilterCount(this.value);
  }

  facet(param: string): FacetState {
    return this.value.facets[param] ?? emptyFacet();
  }

  setQuery(q: string) {
    this.value = { ...this.value, q };
    this.#commit();
  }

  setVisa(on: boolean) {
    this.value = { ...this.value, visa: on };
    this.#commit();
  }

  setSalaryMin(n: number | null) {
    this.value = { ...this.value, salaryMin: n };
    this.#commit();
  }

  setSort(sort: SortField) {
    this.value = { ...this.value, sort };
    this.#commit();
  }

  /** Toggle a facet between match-all (AND) and match-any (OR) of its values. */
  setMatchAll(param: string, on: boolean) {
    this.#setFacet(param, { ...this.facet(param), matchAll: on });
  }

  /** Add the value to a facet if absent, remove it if present (pills). */
  toggle(param: string, v: string) {
    const st = this.facet(param);
    const has = st.values.includes(v);
    this.#setFacet(param, { ...st, values: has ? st.values.filter((x) => x !== v) : [...st.values, v] });
  }

  /** Add a token to a facet (token inputs); no-op on blank or duplicate. */
  add(param: string, raw: string) {
    const v = raw.trim();
    const st = this.facet(param);
    if (!v || st.values.includes(v)) return;
    this.#setFacet(param, { ...st, values: [...st.values, v] });
  }

  remove(param: string, v: string) {
    const st = this.facet(param);
    this.#setFacet(param, { ...st, values: st.values.filter((x) => x !== v) });
  }

  /** Reset a single facet (values + exclude mode) — the per-section clear. */
  clearFacet(param: string) {
    this.#setFacet(param, emptyFacet());
  }

  /** Switch a facet between include and exclude mode (the "Исключить" link). */
  setExclude(param: string, exclude: boolean) {
    this.#setFacet(param, { ...this.facet(param), exclude });
  }

  clear() {
    this.value = emptyFilters();
    this.#commit();
  }

  /** Replace the entire filter state from a saved query string and mirror it to
   *  the URL — applies a saved search. Unknown params are ignored by
   *  filtersFromParams, so a stale save degrades to a partial filter. */
  apply(query: string) {
    this.value = filtersFromParams(new URLSearchParams(query));
    this.#commit();
  }

  /** Re-read filters from the current URL (browser back/forward). No-op when
   *  already in sync, which also breaks the write-back loop after our own
   *  setQuery. */
  syncFromUrl() {
    const current = page.url.searchParams;
    if (current.toString() === filtersToParams(this.value).toString()) return;
    this.value = filtersFromParams(current);
  }

  #setFacet(param: string, st: FacetState) {
    this.value = { ...this.value, facets: { ...this.value.facets, [param]: st } };
    this.#commit();
  }

  /** Mirror the current filters into the URL in place — `replaceState` updates
   *  the address bar and history without re-running `load`, so toggling a facet
   *  doesn't round-trip to the server; the view re-searches reactively. Called
   *  synchronously from each mutation (never a separate effect) so a controlled
   *  input never reverts mid-keystroke. Browser-only (mutations are user events). */
  #commit() {
    const qs = filtersToParams(this.value).toString();
    replaceState(page.url.pathname + (qs ? `?${qs}` : ''), {});
  }
}
