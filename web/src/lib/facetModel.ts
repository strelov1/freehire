// The pure job-filter model: the facet/filter types, their URL <-> state
// (de)serialization; the per-value sign transitions are in facetSign.ts and the sort
// order in jobSort.ts. No SvelteKit or Svelte
// runes here, so this module is unit-testable in plain Node and importable from
// both the reactive store (filters.ts) and the staged-edit surface. Param names
// match what the search API (GET /api/v1/jobs/search) expects, including the
// `<param>_exclude` and `<param>_mode=and` conventions.

import { CLEARANCE_UNSPECIFIED, FACETS, REGION_UNSPECIFIED, dynamicLabel } from './facets';
import { facetSetSign } from './facetSign';
import { defaultSortFor, effectiveSort, type JobSort } from './jobSort';
import { REGION_LABELS } from './labels';
import type { FacetState, JobFilters } from './jobFilters';
export type { FacetState, JobFilters } from './jobFilters';

/** Strips a title suggestion's Meilisearch quoting wrapper (see `apiSuggestions.ts`'s
 *  `quoteForTitleSearch`) from a query before showing it to a person: a filter chip,
 *  the header search box, an analytics event. The wrapper is a query-construction
 *  detail — it decides how Meilisearch matches `q`, not what the visitor typed or
 *  should read back. Mirrors `demandKey` in internal/api/handler/search.go, which
 *  strips the same wrapper for a different reason (demand-tracking key
 *  normalisation). */
export function displayQuery(q: string): string {
  return q.length >= 2 && q.startsWith('"') && q.endsWith('"') ? q.slice(1, -1) : q;
}

/** The `sort` values the search endpoint accepts, keyed by our vocabulary. `relevance`
 *  is absent on purpose: the endpoint spells it as no `sort` parameter at all, and
 *  inventing a wire value for it would need a handler branch to mean the same thing —
 *  so a sort with no entry here is one that serializes to nothing. */
const SORT_PARAM: Partial<Record<JobSort, string>> = {
  newest: 'posted_at',
  views: 'view_count',
  match: 'match',
};

/** SORT_PARAM inverted, so the two directions cannot drift. */
const SORT_FROM_PARAM: Record<string, JobSort> = Object.fromEntries(
  Object.entries(SORT_PARAM).map(([sort, param]) => [param, sort as JobSort]),
);

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
    hideAIInterview: false,
    autoApplyAvailable: false,
    formerEmployers: false,
    watchlist: false,
    applyQuestionsMax: null,
    minSkillMatch: null,
    salaryMin: null,
    salaryMax: null,
    postedWithinDays: null,
    openWithinDays: null,
    experienceYearsMax: null,
    experienceIncludeUnspecified: false,
    experienceOnlySeniorityUnspecified: false,
    seniorityIncludeUnspecified: false,
    sort: null,
    qFields: null,
    hideSeen: false,
    workModeRemoteAnywhere: false,
    workModeRemoteRegions: [],
    workModeRemoteCountries: [],
  };
}

// ---- URL serialization ----

/** Serialize filters to URL query params (the shape the search API reads). */
export function filtersToParams(f: JobFilters): URLSearchParams {
  const p = new URLSearchParams();
  if (f.q) p.set('q', f.q);
  if (f.qFields?.length) p.set('q_fields', f.qFields.join(','));
  for (const def of FACETS) {
    const st = f.facets[def.param];
    if (!st) continue;
    if (st.include.length > 0) p.set(def.param, st.include.join(','));
    if (st.exclude.length > 0) p.set(`${def.param}_exclude`, st.exclude.join(','));
    // AND-mode is per facet and only meaningful with more than one included value.
    if (st.matchAll && st.include.length > 1) p.set(`${def.param}_mode`, 'and');
  }
  if (f.visa) p.set('visa_sponsorship', 'true');
  if (f.hideAIInterview) p.set('ai_interview', 'false');
  if (f.autoApplyAvailable) p.set('auto_apply_available', 'true');
  if (f.formerEmployers) p.set('former_employers', 'true');
  if (f.watchlist) p.set('watchlist', 'true');
  if (f.applyQuestionsMax != null) p.set('apply_questions_max', String(f.applyQuestionsMax));
  if (f.minSkillMatch != null) p.set('min_skill_match', String(f.minSkillMatch));
  if (f.salaryMin != null) p.set('salary_min', String(f.salaryMin));
  if (f.salaryMax != null) p.set('salary_max', String(f.salaryMax));
  if (f.postedWithinDays != null) p.set('posted_within_days', String(f.postedWithinDays));
  if (f.openWithinDays != null) p.set('open_within_days', String(f.openWithinDays));
  if (f.experienceYearsMax != null) {
    p.set('experience_years_max', String(f.experienceYearsMax));
    if (f.experienceIncludeUnspecified) p.set('experience_include_unspecified', 'true');
    if (f.experienceOnlySeniorityUnspecified) p.set('experience_only_seniority_unspecified', 'true');
  }
  if (f.seniorityIncludeUnspecified && (f.facets.seniority?.include.length ?? 0) > 0) {
    p.set('seniority_include_unspecified', 'true');
  }
  // The default is written as the ABSENCE of the param, which is how the endpoint
  // spells both of its own defaults — so a browse feed and a relevance-ranked search
  // both serialize clean, and `newest` under a query becomes explicit instead of
  // silently meaning relevance.
  const sort = effectiveSort(f);
  const sortParam = SORT_PARAM[sort];
  if (sortParam && sort !== defaultSortFor(f.q)) p.set('sort', sortParam);
  if (f.hideSeen) p.set('hide_seen', 'true');
  if (f.workModeRemoteAnywhere) {
    p.set('work_mode_remote_anywhere', 'true');
    if (f.workModeRemoteRegions.length) p.set('work_mode_remote_regions', f.workModeRemoteRegions.join(','));
    if (f.workModeRemoteCountries.length) p.set('work_mode_remote_countries', f.workModeRemoteCountries.join(','));
  }
  return p;
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

/** A raw salary bound read exactly as the server reads it, or null when the server
 *  ignores it. No range check, because the backend applies none: `salary_max=0` does
 *  narrow the list (to nothing), and reading it as unset here would hide the bound
 *  that emptied it. */
function salaryBound(raw: string | null): number | null {
  return raw !== null && ATOI.test(raw) ? Number(raw) : null;
}

/** A non-negative whole number, or null for anything else, read the way the server
 *  reads the params that TRIM first: `apply_questions_max` and `min_skill_match`. A
 *  param the server reads untrimmed (`experience_years_max`) must not use this — see
 *  `salaryBound`. The syntax is Go's Atoi (ATOI), not `Number`, which also reads `0x0`,
 *  `1e1` and a blank string — `apply_questions_max=0x0` would otherwise become a bound
 *  the server never applies. */
function nonNegativeInt(raw: string | null): number | null {
  const trimmed = raw?.trim() ?? '';
  if (!ATOI.test(trimmed)) return null;
  const n = Number(trimmed);
  return n >= 0 ? n : null;
}

/** Parse filters back from URL query params. Include and exclude are independent
 *  sets; if a value appears in both (a malformed or legacy link), exclude wins and
 *  it is dropped from include so a value carries exactly one sign. */
export function filtersFromParams(p: URLSearchParams): JobFilters {
  const f = emptyFilters();
  f.q = p.get('q') ?? '';
  const qFields = splitParamValues(p.getAll('q_fields'));
  f.qFields = qFields.length > 0 ? qFields : null;
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
  f.hideAIInterview = p.get('ai_interview') === 'false';
  f.autoApplyAvailable = p.get('auto_apply_available') === 'true';
  f.formerEmployers = p.get('former_employers') === 'true';
  f.watchlist = p.get('watchlist') === 'true';
  f.applyQuestionsMax = nonNegativeInt(p.get('apply_questions_max'));
  const minSkillMatch = nonNegativeInt(p.get('min_skill_match'));
  f.minSkillMatch = minSkillMatch != null && minSkillMatch > 0 && minSkillMatch <= 100 ? minSkillMatch : null;
  readLegacyClearance(p, f.facets);
  f.salaryMin = salaryBound(p.get('salary_min'));
  f.salaryMax = salaryBound(p.get('salary_max'));
  // Both date bounds are a positive whole number of days; anything else (absent,
  // zero, negative, non-numeric) reads as "any age", matching the backend's own guard.
  f.postedWithinDays = positiveDays(p.get('posted_within_days'));
  f.openWithinDays = positiveDays(p.get('open_within_days'));
  // Zero IS a bound here — it selects the postings stating no prior experience is
  // required — so the guard admits it and rejects only what cannot be a year count.
  // The server does NOT trim this one (a bare atoiOK), so neither may we: `%203` is no
  // bound there, and a trimmed read would show a chip over an unbounded list.
  const experienceYearsMax = salaryBound(p.get('experience_years_max'));
  f.experienceYearsMax = experienceYearsMax != null && experienceYearsMax >= 0 ? experienceYearsMax : null;
  f.experienceIncludeUnspecified = p.get('experience_include_unspecified') === 'true';
  f.experienceOnlySeniorityUnspecified = p.get('experience_only_seniority_unspecified') === 'true';
  f.seniorityIncludeUnspecified = p.get('seniority_include_unspecified') === 'true';
  // Anything but a recognized value reads as the contextual default — including the
  // retired `sort=cv` and the endpoint's `created_at`, which the browse UI does not
  // offer. Shared links and saved searches still carry old sort params, and the same
  // rule the backend applies (ignore, never refuse) has to hold here or the two would
  // disagree about what a stale link means. Reads `f.q`, so it must follow the line
  // that sets it.
  f.sort = SORT_FROM_PARAM[p.get('sort') ?? ''] ?? null;
  f.hideSeen = p.get('hide_seen') === 'true';
  // Read like the server reads it (trimmed, case-insensitive), so a hand-written TRUE
  // does not silently drop a modifier the server would apply.
  f.workModeRemoteAnywhere = p.get('work_mode_remote_anywhere')?.trim().toLowerCase() === 'true';
  const list = (key: string) =>
    p
      .getAll(key)
      .flatMap((v) => v.split(','))
      .map((v) => v.trim())
      .filter(Boolean);
  if (f.workModeRemoteAnywhere) {
    f.workModeRemoteRegions = list('work_mode_remote_regions');
    f.workModeRemoteCountries = list('work_mode_remote_countries');
  }
  return f;
}

/** Whether the remote-anywhere modifier actually changes the answer for this filter
 *  set — the server's precondition (`remoteAnywhereActive` in
 *  internal/search/search/query_filter.go), mirrored so the checkbox shows only when
 *  it has something to do and the count only rises when it does. */
export function appliesWorkModeRemoteAnywhere(f: JobFilters): boolean {
  const wm = f.facets.work_mode;
  if (!wm || wm.matchAll) return false; // the server ignores it under work_mode_mode=and
  // Folded like the server's workModeValues, so `Remote` from a hand-written link
  // shows the checkbox and chip whenever the server applies the modifier.
  const modes = wm.include.map((m) => m.trim().toLowerCase());
  const hasLocation = LOCATION_FACET_PARAMS.some((p) => (f.facets[p]?.include ?? []).some((v) => knownLocation(p, v)));
  return modes.includes('remote') && modes.some(isPhysicalMode) && hasLocation;
}

/** The modifier's chip and checkbox text: where the remote arm reaches. */
export function workModeRemoteLabel(f: JobFilters): string {
  const reach = [
    ...f.workModeRemoteRegions.map((v) => REGION_LABELS[v.trim().toLowerCase()] ?? v),
    ...f.workModeRemoteCountries.map((v) => dynamicLabel('countries', v)),
  ];
  return reach.length ? `Remote within ${reach.join(', ')}` : 'Remote from anywhere';
}

/** The location facets that share one ORed group on the server (`locationFacets`). */
const LOCATION_FACET_PARAMS = ['regions', 'countries', 'cities', 'metros'];

export const isPhysicalMode = (m: string) => m === 'hybrid' || m === 'onsite';

/** `regions` is the one closed location facet: like the server's
 *  `hasLocationInclude`, an unknown region (a hand-edited link) is not a location. */
function knownLocation(param: string, value: string): boolean {
  if (param !== 'regions') return true;
  const v = value.trim().toLowerCase();
  return v === REGION_UNSPECIFIED || v in REGION_LABELS;
}

/** The security-clearance control used to be a three-state toggle serialized as
 *  `requires_clearance=false|true`, and saved searches, alerts and shared links still
 *  carry it. It is now the `clearance_levels` chips, whose reserved `none` ("no
 *  explicit reference to clearance") the backend answers with exactly the old
 *  `requires_clearance=false` (search.ClearanceLevelUnspecified) — so the old link
 *  reads as that chip: `false` includes it, `true` excludes it (every posting that
 *  states a clearance). The URL is rewritten in the new form on the next change, and
 *  the backend still honours the old param for every client that never moves.
 *
 *  A link carrying both forms keeps what `clearance_levels` says about `none`, and an
 *  unrecognised value is ignored rather than guessed at. */
function readLegacyClearance(p: URLSearchParams, facets: Record<string, FacetState>): void {
  const legacy = p.get('requires_clearance');
  if (legacy !== 'false' && legacy !== 'true') return;
  const st = facets.clearance_levels ?? emptyFacet();
  if (st.include.includes(CLEARANCE_UNSPECIFIED) || st.exclude.includes(CLEARANCE_UNSPECIFIED)) return;
  facets.clearance_levels =
    legacy === 'false'
      ? { ...st, include: [...st.include, CLEARANCE_UNSPECIFIED] }
      : { ...st, exclude: [...st.exclude, CLEARANCE_UNSPECIFIED] };
}

/** Total selected facet values (plus visa/salary/freshness) — drives the mobile badge. */
export function activeFilterCount(f: JobFilters): number {
  let n = 0;
  for (const def of FACETS) {
    const st = f.facets[def.param];
    if (st) n += st.include.length + st.exclude.length;
  }
  if (f.visa) n += 1;
  if (f.hideAIInterview) n += 1;
  if (f.autoApplyAvailable) n += 1;
  if (f.formerEmployers) n += 1;
  if (f.watchlist) n += 1;
  if (f.applyQuestionsMax != null) n += 1;
  if (f.minSkillMatch != null) n += 1;
  // One control and one summary chip, so one count whichever ends are set.
  if (f.salaryMin != null || f.salaryMax != null) n += 1;
  if (f.postedWithinDays != null) n += 1;
  if (f.openWithinDays != null) n += 1;
  if (f.experienceYearsMax != null) n += 1;
  // The modifier counts only when it changes the answer — off or inert, it narrows
  // nothing and a badge for it would lie.
  if (f.workModeRemoteAnywhere && appliesWorkModeRemoteAnywhere(f)) n += 1;
  return n;
}

/** Apply everything a chosen suggestion names, in ONE filter change.
 *
 *  A completion from the suggestions endpoint composes a phrase, so it can name a role
 *  AND a company at once ("Senior Software Engineer Google"). Applying them one at a
 *  time would reload the list per part and leave it briefly filtered by half a query,
 *  and the intermediate URL would be a search nobody asked for.
 *
 *  The typed text is replaced rather than kept: the parts ARE what was typed, resolved.
 *  A `title` part carries it back as `q`, since no facet spells "Product Owner".
 *
 *  `qFields` is replaced too, not merged: a previous suggestion's title-field
 *  restriction (see `JobFilters.qFields`) must not silently carry over onto a new
 *  suggestion that names no title of its own, so a missing/empty argument here
 *  clears it rather than leaving the prior value in place. */
export function filtersWithParts(
  f: JobFilters,
  parts: readonly (readonly [param: string, value: string])[],
  q: string,
  qFields: readonly string[] | null = null,
): JobFilters {
  const facets = { ...f.facets };
  for (const [param, value] of parts) {
    facets[param] = facetSetSign(facets[param] ?? emptyFacet(), value, 'include');
  }
  return { ...f, q, qFields: qFields?.length ? [...qFields] : null, facets };
}
