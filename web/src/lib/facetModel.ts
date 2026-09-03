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

/** The clearance control's three states. `any` writes no param at all, so the default
 *  listing is untouched. */
export type ClearanceFilter = 'any' | 'hide' | 'only';

export interface JobFilters {
  q: string;
  /** Facet state keyed by the facet's query param (see FACETS). */
  facets: Record<string, FacetState>;
  visa: boolean;
  /** What to do about postings that state a government security-clearance requirement
   *  (UK SC/DV, US Secret/TS-SCI, AU NV1). Three states rather than a boolean because
   *  the facet answers two different people: someone who cannot hold a clearance wants
   *  these gone, someone who holds one wants nothing else.
   *
   *  Serialized as `requires_clearance`, whose value names the FACET while this names
   *  the INTENT — so `hide` writes `false` and `only` writes `true`. An unrecognised
   *  value in a hand-edited link reads as `any` rather than leaving the control in a
   *  state it cannot render. */
  clearance: ClearanceFilter;
  salaryMin: number | null;
  /** Freshness: keep only jobs posted within the last N days (null = any age).
   *  Serialized as `posted_within_days`; the backend turns it into a posted_ts
   *  range filter relative to request time.
   *
   *  This bounds the date the SOURCE states. Some boards restate it on every crawl,
   *  so a posting open for months can satisfy a three-day bound here — see
   *  `openWithinDays`, which is the bound that cannot be rewritten from outside. */
  postedWithinDays: number | null;
  /** Keep only jobs first seen within the last N days (null = any age). Serialized as
   *  `open_within_days`; the backend turns it into a created_ts range filter.
   *
   *  Independent of `postedWithinDays` and not a substitute for it: one asks how long
   *  the posting has been in the catalogue, the other what date the board claims. Both
   *  may be set, and they narrow the list together. */
  openWithinDays: number | null;
  /** Keep only jobs asking for at most this many years of experience (null = any).
   *  `0` is a real bound — the jobs stating no prior experience is required — so
   *  every read of this field must test for null, never for falsiness. */
  experienceYearsMax: number | null;
  /** Feed ordering, or `null` for "the caller has not chosen one".
   *
   *  `relevance` is the engine's own ranking, `newest` is freshest first, `match` ranks
   *  by how well a vacancy's skills overlap the signed-in caller's profile. The server
   *  degrades `match` to its default for anyone it cannot serve it to — anonymous, no
   *  profile, no skills — so this is never gated client-side beyond hiding the option.
   *
   *  The `null` matters because the DEFAULT depends on `q` (see defaultSortFor) while
   *  `q` changes under the ordering's feet. Storing the resolved default instead would
   *  make "the browse feed defaulted to newest" indistinguishable from "the caller
   *  asked for newest", so typing into the search box would carry `sort=posted_at`
   *  into a text search and date-order it — the exact outcome design.md rejects. Read
   *  this through effectiveSort, never directly. */
  sort: JobSort | null;
}

/** The feed's ordering vocabulary. Deliberately short: this is not a general sort
 *  control (the API also accepts created_at and the salary bounds), it is the two
 *  orderings the endpoint defaults between plus the profile-match feed. */
export type JobSort = 'relevance' | 'newest' | 'match';

/** The ordering the endpoint applies when a request carries no `sort` at all:
 *  relevance under query text, posting date without it (see `searchSort` in
 *  internal/api/handler/search.go). The client mirrors it rather than restating it,
 *  so "the default" and "what the server does with no param" cannot drift — which is
 *  also why serializing the default means writing nothing. */
export function defaultSortFor(q: string): JobSort {
  return q ? 'relevance' : 'newest';
}

/** The ordering a filter set actually resolves to.
 *
 *  An unchosen ordering resolves to the contextual default, and an explicit
 *  `relevance` collapses to the browse default once the query is cleared — it has
 *  nothing left to rank against. This is a pure function, not an effect that rewrites
 *  the stored value, because BOTH the sort control and filtersToParams need the answer
 *  and a second copy of the rule is a second answer. */
export function effectiveSort(f: JobFilters): JobSort {
  const sort = f.sort ?? defaultSortFor(f.q);
  return sort === 'relevance' && !f.q ? 'newest' : sort;
}

/** The `sort` values the search endpoint accepts, keyed by our vocabulary. `relevance`
 *  is absent on purpose: the endpoint spells it as no `sort` parameter at all, and
 *  inventing a wire value for it would need a handler branch to mean the same thing —
 *  so a sort with no entry here is one that serializes to nothing. */
const SORT_PARAM: Partial<Record<JobSort, string>> = { newest: 'posted_at', match: 'match' };

/** SORT_PARAM inverted, so the two directions cannot drift. */
const SORT_FROM_PARAM: Record<string, JobSort> = Object.fromEntries(
  Object.entries(SORT_PARAM).map(([sort, param]) => [param, sort as JobSort]),
);

const SORT_LABEL: Record<JobSort, string> = {
  relevance: 'Relevance',
  newest: 'Newest',
  match: 'Best match',
};

export interface SortOption {
  value: JobSort;
  label: string;
}

/** The orderings a caller can actually choose between, in display order.
 *
 *  `relevance` needs query text to rank against and `match` needs a profile the caller
 *  has (plus the runtime flag the view resolves); `newest` always applies. The rule is
 *  per OPTION rather than per control — gating the whole select on the match
 *  precondition, as it was, took `newest` down with it, so a signed-out visitor
 *  searching by text had no way to reach the freshest-first ordering.
 *
 *  It lives here, not in the view, for the reason effectiveSort does: it is pure, it
 *  decides what the user sees, and in the component nothing could test it. */
export function sortOptionsFor(q: string, matchAvailable: boolean): SortOption[] {
  const values: JobSort[] = [...(q ? (['relevance'] as const) : []), 'newest', ...(matchAvailable ? (['match'] as const) : [])];
  return values.map((value) => ({ value, label: SORT_LABEL[value] }));
}

/** The option the control shows as selected.
 *
 *  Normally the effective ordering — but a shared `?sort=match` link opened by someone
 *  it cannot be served to resolves to an ordering that is not on offer. The param stays
 *  (the server degrades it rather than refusing, and signing in should just work), so
 *  the control instead names what the server will ACTUALLY serve that caller. A select
 *  whose value matches no option renders blank, which would put an empty control over a
 *  live ordering. */
export function selectedSortFor(f: JobFilters, matchAvailable: boolean): JobSort {
  const sort = effectiveSort(f);
  const offered = sortOptionsFor(f.q, matchAvailable);
  return offered.some((o) => o.value === sort) ? sort : defaultSortFor(f.q);
}

/** Splits every raw query value on comma and flattens the result, dropping
 *  empty fragments (a stray comma) — so a repeated key (`skills=go&skills=react`)
 *  and a comma-joined value (`skills=go,react`) resolve to the same values.
 *  Mirrors the backend's `splitFacetValues` (internal/search/query_filter.go). */
function splitParamValues(raw: string[]): string[] {
  return raw.flatMap((v) => v.split(',')).filter((v) => v !== '');
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
  return {
    q: '',
    facets: emptyFacets(),
    visa: false,
    clearance: 'any',
    salaryMin: null,
    postedWithinDays: null,
    openWithinDays: null,
    experienceYearsMax: null,
    sort: null,
  };
}

// ---- URL serialization ----

/** Serialize filters to URL query params (the shape the search API reads). */
export function filtersToParams(f: JobFilters): URLSearchParams {
  const p = new URLSearchParams();
  if (f.q) p.set('q', f.q);
  for (const def of FACETS) {
    const st = f.facets[def.param];
    if (!st) continue;
    if (st.include.length > 0) p.set(def.param, st.include.join(','));
    if (st.exclude.length > 0) p.set(`${def.param}_exclude`, st.exclude.join(','));
    // AND-mode is per facet and only meaningful with more than one included value.
    if (st.matchAll && st.include.length > 1) p.set(`${def.param}_mode`, 'and');
  }
  if (f.visa) p.set('visa_sponsorship', 'true');
  if (f.clearance === 'hide') p.set('requires_clearance', 'false');
  if (f.clearance === 'only') p.set('requires_clearance', 'true');
  if (f.salaryMin != null) p.set('salary_min', String(f.salaryMin));
  if (f.postedWithinDays != null) p.set('posted_within_days', String(f.postedWithinDays));
  if (f.openWithinDays != null) p.set('open_within_days', String(f.openWithinDays));
  if (f.experienceYearsMax != null) p.set('experience_years_max', String(f.experienceYearsMax));
  // The default is written as the ABSENCE of the param, which is how the endpoint
  // spells both of its own defaults — so a browse feed and a relevance-ranked search
  // both serialize clean, and `newest` under a query becomes explicit instead of
  // silently meaning relevance.
  const sort = effectiveSort(f);
  const sortParam = SORT_PARAM[sort];
  if (sortParam && sort !== defaultSortFor(f.q)) p.set('sort', sortParam);
  return p;
}

/** Whether a general facet response measured under `scope` also answers for the role
 *  distribution, which is measured under the same scope minus the text query.
 *
 *  The two coincide exactly when there is no text query to separate them — and then
 *  they are not merely equivalent but identical, because `/jobs/facets` with no
 *  `facets=` counts every facet, `role` among them. Measured against prod on
 *  2026-08-21: `?regions=latam,global` and `?regions=latam,global&facets=role`
 *  returned byte-identical `role` maps (867 values), while adding `q=python` cut the
 *  general response's `role` to 209 — which is exactly why the role distribution has
 *  its own scope in the first place.
 *
 *  So this is the test for "the dedicated role request would be a second copy of a
 *  response we already have". An empty `q` counts as no query: `filtersToParams` only
 *  writes the param when the string is non-empty, and a hand-built `?q=` means the
 *  same thing to the API. */
export function generalCountsCoverRole(scope: URLSearchParams): boolean {
  return !scope.get('q');
}

/** The furthest back either date bound reaches, mirroring `maxWithinDays` in
 *  internal/search/search. A century is past anything in the catalogue; the ceiling is
 *  there because Go's day-to-duration arithmetic wraps above ~106,751 days, and a bound
 *  the server drops must not leave a chip here claiming it applied. */
const MAX_WITHIN_DAYS = 36500;

/** Matches exactly what Go's `strconv.Atoi` accepts: an optional sign, then digits.
 *
 *  `Number()` is more generous — it reads `1e2` as 100, `1.0` as 1 and `0x10` as 16,
 *  none of which Atoi accepts. Reading a param one way here and another way on the
 *  server is how `?open_within_days=1e2` came to show an active "Last 100 days" chip
 *  over a list the server had never bounded. Leading zeros and a leading `+` DO parse
 *  in Go, so they parse here too: agreement is the point, not strictness. */
const ATOI = /^[+-]?\d+$/;

/** A raw param read as a whole number of days the search will actually honour, or null
 *  for "no bound".
 *
 *  Shared by the two date bounds so the rule is written once. A value that cannot be a
 *  day count — absent, blank, mis-spelled, zero, negative, or further back than the
 *  server reaches — imposes NO restriction rather than narrowing the list to nothing,
 *  which is what the backend does with the same params. */
function positiveDays(raw: string | null): number | null {
  if (raw === null || !ATOI.test(raw)) return null;
  const days = Number(raw);
  return days > 0 && days <= MAX_WITHIN_DAYS ? days : null;
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
    const exclude = [...new Set(splitParamValues(p.getAll(`${def.param}_exclude`)))];
    const excludeSet = new Set(exclude);
    const include = [...new Set(splitParamValues(p.getAll(def.param)))].filter((v) => !excludeSet.has(v));
    const matchAll = p.get(`${def.param}_mode`) === 'and';
    f.facets[def.param] = { include, exclude, matchAll };
  }
  f.visa = p.get('visa_sponsorship') === 'true';
  const clearance = p.get('requires_clearance');
  f.clearance = clearance === 'false' ? 'hide' : clearance === 'true' ? 'only' : 'any';
  const salary = Number(p.get('salary_min'));
  f.salaryMin = p.get('salary_min') && !Number.isNaN(salary) ? salary : null;
  // Both date bounds are a positive whole number of days; anything else (absent,
  // zero, negative, non-numeric) reads as "any age", matching the backend's own guard.
  f.postedWithinDays = positiveDays(p.get('posted_within_days'));
  f.openWithinDays = positiveDays(p.get('open_within_days'));
  // Zero IS a bound here — it selects the postings stating no prior experience is
  // required — so the guard admits it and rejects only what cannot be a year count.
  // The presence test is on the TRIMMED string, not the raw one: `Number('')` and
  // `Number(' ')` are both 0 while `' '` is truthy, so a naive check would turn
  // `?experience_years_max=%20` in a shared link into the entry-level filter.
  const rawYears = p.get('experience_years_max')?.trim() ?? '';
  const years = Number(rawYears);
  f.experienceYearsMax = rawYears !== '' && Number.isInteger(years) && years >= 0 ? years : null;
  // Anything but a recognized value reads as the contextual default — including the
  // retired `sort=cv` and the endpoint's `created_at`, which the browse UI does not
  // offer. Shared links and saved searches still carry old sort params, and the same
  // rule the backend applies (ignore, never refuse) has to hold here or the two would
  // disagree about what a stale link means. Reads `f.q`, so it must follow the line
  // that sets it.
  f.sort = SORT_FROM_PARAM[p.get('sort') ?? ''] ?? null;
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
  if (f.clearance !== 'any') n += 1;
  if (f.salaryMin != null) n += 1;
  if (f.postedWithinDays != null) n += 1;
  if (f.openWithinDays != null) n += 1;
  if (f.experienceYearsMax != null) n += 1;
  return n;
}

/** Normalize a search query string to its canonical form (parse → re-serialize),
 *  so two filter sets that differ only in param order or stale/unknown params
 *  compare equal. Used to detect which saved search matches the current filters. */
export function canonicalQuery(query: string): string {
  return savedSearchQuery(filtersFromParams(new URLSearchParams(query)));
}

/** The saved-search / alert target: the filters as a canonical query string. A thin,
 *  named wrapper over filtersToParams so the saved-search call sites (SavedSearches,
 *  FilterSummary, CompanyFollowButton) express intent ("the comparable query for this
 *  filter set") rather than reaching for the raw serializer. */
export function savedSearchQuery(f: JobFilters): string {
  return filtersToParams(f).toString();
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

/** Choosing a role suggestion under the header search box: turn that role on and drop
 *  the typed text, as ONE new state.
 *
 *  The two belong together. The role tag is derived from the title by a deterministic
 *  dictionary, so it is already the more precise of the two filters — keeping the text
 *  as well would AND a half-typed query against it and return fewer jobs than either
 *  filter alone, which reads as the suggestion breaking the page. Committing them as
 *  one state also means the caller writes once (setNow), which cancels the debounce
 *  the last keystroke left pending instead of racing it.
 *
 *  facetSetSign, not facetAdd: a role sitting in the EXCLUDE set is still offered as a
 *  suggestion — only INCLUDED roles are withheld — and facetAdd treats a value already
 *  in either set as a no-op. Picking such a role would then clear the typed text and
 *  change nothing else, a click that visibly does half its job. Choosing a role you had
 *  excluded is an unambiguous change of mind, so it flips the sign. */
export function filtersWithRole(f: JobFilters, slug: string): JobFilters {
  return {
    ...f,
    q: '',
    facets: { ...f.facets, role: facetSetSign(f.facets.role ?? emptyFacet(), slug, 'include') },
  };
}

/** Build a fresh filter set seeded from a user profile — the reset-and-seed behind
 *  "Apply my profile". Specializations become `category` values, skills become included
 *  `skills` values, and excluded skills become EXCLUDED `skills` values (rendering
 *  `skills_exclude=…` → `skills != "X"`). A skill that is somehow both wanted and avoided
 *  stays wanted (include wins), mirroring the server's overlap rule so the two never
 *  self-cancel. The optional location block flattens into the location facets: work_modes → `work_mode`;
 *  regions from the remote reach ∪ relocation targets; countries from the remote reach ∪
 *  base ∪ relocation targets; cities from the base ∪ relocation targets; and `relocation`
 *  staged as supported+required when the user is open to relocating. The flatten is lossy
 *  (base vs relocation merge) — the filter is a convenience narrowing of "places relevant to
 *  me". Trimming/dedup come free from facetAdd, so unions of overlapping lists are safe. */
export function filtersFromProfile(profile: UserProfile): JobFilters {
  const seed = (values: string[]) => values.reduce(facetAdd, emptyFacet());
  const f = emptyFilters();
  f.facets.category = seed(profile.specializations);
  // Skills: wanted → include, avoided → exclude. Only stage an exclude for a token not
  // already wanted (signOf === 'off'), so a stray overlap keeps the wanted value.
  f.facets.skills = (profile.excluded_skills ?? []).reduce(
    (st, raw) => {
      const v = raw.trim();
      return v && signOf(st, v) === 'off' ? facetSetSign(st, v, 'exclude') : st;
    },
    seed(profile.skills),
  );

  const loc = profile.location_preferences;
  if (loc) {
    // Relocation targets only count when the user is actually open to relocating — `open`
    // gates the whole relocation contribution (targets and the relocation facet alike).
    const reloc = loc.relocation.open ? loc.relocation : { regions: [], countries: [], cities: [] };
    // `base` says where the user LIVES; the facets say where they want the WORK. Those
    // coincide only for someone who accepts physical work — the job has to be commutable
    // — so base is folded in for them and withheld from everyone else. Seeding a
    // remote-only candidate's home country as a job-country filter would narrow their
    // search to the one country they least need the job to be in.
    //
    // This gate used to be implicit: the profile form collected `base` from on-site and
    // hybrid users only, and dropped it on save for everyone else. Now that the form asks
    // every user where they are, the gate has to be stated here instead.
    const wantsPhysical = (loc.work_modes ?? []).some((m) => m === 'onsite' || m === 'hybrid');
    const base = wantsPhysical ? loc.base : {};
    f.facets.work_mode = seed(loc.work_modes ?? []);
    f.facets.regions = seed([...(loc.remote.regions ?? []), ...(reloc.regions ?? [])]);
    f.facets.countries = seed([
      ...(loc.remote.countries ?? []),
      ...(base.country ? [base.country] : []),
      ...(reloc.countries ?? []),
    ]);
    f.facets.cities = seed([
      ...(base.city ? [base.city] : []),
      ...(reloc.cities ?? []),
    ]);
    if (loc.relocation.open) f.facets.relocation = seed(['supported', 'required']);
  }
  return f;
}
