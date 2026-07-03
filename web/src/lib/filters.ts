// Job search filters: the model, its URL <-> state (de)serialization, and a
// reactive store that mirrors the filters into the URL query so they survive
// reloads, sharing, and back/forward. Param names match what the search API
// (GET /api/v1/jobs/search) expects, including the `<param>_exclude` and
// `<param>_mode=and` conventions.

import { FACETS } from './facets';
import { UrlSyncedState } from './urlSynced.svelte';

/** One facet's selection: the chosen values, whether it filters by inclusion or
 *  exclusion, and (for facets that allow it) whether selected values are ANDed
 *  (match all) instead of ORed (match any). */
export interface FacetState {
  values: string[];
  exclude: boolean;
  matchAll: boolean;
}

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
  return { values: [], exclude: false, matchAll: false };
}

function emptyFacets(): Record<string, FacetState> {
  const out: Record<string, FacetState> = {};
  for (const f of FACETS) out[f.param] = emptyFacet();
  return out;
}

export function emptyFilters(): JobFilters {
  return { q: '', facets: emptyFacets(), visa: false, salaryMin: null, postedWithinDays: null, sort: DEFAULT_SORT };
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
  if (f.postedWithinDays != null) p.set('posted_within_days', String(f.postedWithinDays));
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
    // URL params aren't guaranteed unique (shared/edited links, crawlers), but a
    // facet's values are a set — `add`/`toggle` enforce that on user input, so the
    // URL parse must too. A repeated value otherwise reaches TokenInput, which keys
    // each chip by its value, and Svelte throws `each_key_duplicate` on hydration.
    const exclude = [...new Set(p.getAll(`${def.param}_exclude`))];
    const include = [...new Set(p.getAll(def.param))];
    const matchAll = p.get(`${def.param}_mode`) === 'and';
    if (exclude.length > 0) f.facets[def.param] = { values: exclude, exclude: true, matchAll };
    else if (include.length > 0) f.facets[def.param] = { values: include, exclude: false, matchAll };
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

/** Total selected facet values (plus visa/salary) — drives the mobile badge. */
export function activeFilterCount(f: JobFilters): number {
  let n = 0;
  for (const def of FACETS) n += f.facets[def.param]?.values.length ?? 0;
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

/** Reactive job filters mirrored into the URL. A thin wrapper over the shared
 *  `UrlSyncedState` primitive: it owns the job-specific shape (facets/visa/salary/
 *  sort) and the discrete-vs-continuous policy, while the primitive owns the
 *  state<->URL transport and the reload debounce. Read `value` to drive inputs and
 *  `applied` (the debounced snapshot) to drive the data reload. */
export class FilterStore {
  #url: UrlSyncedState<JobFilters>;

  /** Seed from the current URL params (passed by the view from `page.url`), so
   *  the same filters render on the server and hydrate on the client. */
  constructor(initial?: URLSearchParams) {
    this.#url = new UrlSyncedState<JobFilters>(initial ?? new URLSearchParams(), {
      parse: filtersFromParams,
      serialize: filtersToParams,
    });
  }

  /** Live filters — bind inputs to this. */
  get value(): JobFilters {
    return this.#url.value;
  }

  /** Debounced filters — drive the list/counts reload off this. */
  get applied(): JobFilters {
    return this.#url.applied;
  }

  get active(): number {
    return activeFilterCount(this.#url.value);
  }

  facet(param: string): FacetState {
    return this.#url.value.facets[param] ?? emptyFacet();
  }

  // Continuous inputs (typed/dragged): debounce the reload via setSoon.
  setQuery(q: string) {
    this.#url.setSoon({ ...this.#url.value, q });
  }

  setSalaryMin(n: number | null) {
    this.#url.setSoon({ ...this.#url.value, salaryMin: n });
  }

  // Dragged like the salary slider (a continuous gesture across snap points), so
  // it debounces the reload via setSoon — the URL still updates immediately.
  setPostedWithinDays(n: number | null) {
    this.#url.setSoon({ ...this.#url.value, postedWithinDays: n });
  }

  // Discrete inputs (clicked/toggled): apply immediately via setNow.
  setVisa(on: boolean) {
    this.#url.setNow({ ...this.#url.value, visa: on });
  }

  setSort(sort: SortField) {
    this.#url.setNow({ ...this.#url.value, sort });
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
    this.#url.setNow(emptyFilters());
  }

  /** Replace the entire filter state from a saved query string and mirror it to
   *  the URL — applies a saved search. Unknown params are ignored by
   *  filtersFromParams, so a stale save degrades to a partial filter. */
  apply(query: string) {
    this.#url.setNow(filtersFromParams(new URLSearchParams(query)));
  }

  /** Re-read filters from the current URL (browser back/forward). */
  syncFromUrl() {
    this.#url.syncFromUrl();
  }

  /** Cancel any pending debounced reload — call from the owning view's cleanup. */
  dispose() {
    this.#url.dispose();
  }

  #setFacet(param: string, st: FacetState) {
    this.#url.setNow({ ...this.#url.value, facets: { ...this.#url.value.facets, [param]: st } });
  }
}
