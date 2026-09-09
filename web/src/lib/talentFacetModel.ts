// The pure model behind the Talent Network catalogue's filters: the type, its
// serialisation, and the mutators a facet control needs. No `$app`, no reactivity — the
// reactive wrapper is talentFilters.ts, exactly as companyFacetModel.ts sits under
// CompanyFilterStore.
//
// Facets live in a MAP keyed by param, not in named fields, because that is the shape a
// facet control asks for: FacetSection is handed a param string and calls
// store.facet(param). Named fields would need a switch translating one into the other,
// and a switch is a second list to keep in step with this one.

// Every facet the catalogue filters on, GENERATED from the Go side's
// `talentnetwork.FacetParams` — the API is the authority on what it reads, and a
// hand-copied list here would drift silently: a param the panes offer but the API does
// not read is not an error, it is reported in `meta.ignored_params` and the answer WIDENS,
// so the visitor is shown more candidates than their own chips claim.
//
// Iterating it also fixes the SERIALISATION order, so two filters selecting the same
// things produce the same string — which is what makes a shared link stable.
import { TALENT_FACET_PARAMS } from '$lib/generated/contracts';

/** The catalogue's whole filter vocabulary. Every value is a closed-vocabulary term or a
 *  number; there is no free-text search, because a card carries no free text to search. */
export interface TalentFilters {
  facets: Record<string, string[]>;
  minYears?: number;
  limit?: number;
  offset?: number;
}

// The API's own bounds, mirrored so the UI never forwards a value the backend will refuse
// to read. The backend is the authority; these are copies, and each names what it mirrors.
//
// Getting one wrong is not cosmetic. A value the API cannot read is not an error there —
// it is reported in `meta.ignored_params` and the answer WIDENS — so a bad copy here shows
// a visitor an unfiltered catalogue while their filter chips claim otherwise.

/** internal/candidate/talentnetwork/query.go: maxLimit. */
export const MAX_LIMIT = 100;

/** internal/candidate/talentnetwork/query.go: maxFilterTerms. */
export const MAX_FILTER_TERMS = 25;

/** internal/candidate/talentnetwork/query.go: maxYears. Mirrored exactly, not narrowed to
 *  something that looks sensible: a bound tighter here drops a filter the API would have
 *  read, and the visitor sees a wider catalogue than their own chips claim — with nothing
 *  in meta.ignored_params to explain it, because the API never saw the parameter. */
const MAX_YEARS = 60;

/** The widest OFFSET the API's int32 argument can carry. */
const MAX_OFFSET = 2_147_483_647;

/** The IANA timezone regions a candidate can actually be in. Antarctica and the
 *  single-city oddities are left out: the row is a control, not a census, and a chip
 *  nobody will ever match is a chip in the way of the ones they will. */
export const TIMEZONE_REGIONS = [
  'Africa',
  'America',
  'Asia',
  'Atlantic',
  'Australia',
  'Europe',
  'Indian',
  'Pacific',
] as const;

/** The experience thresholds the control offers. Round numbers a person thinks in, not an
 *  even split of the range — "at least five years" is a thing somebody means. */
export const YEAR_THRESHOLDS = [2, 5, 8, 12] as const;

export function emptyTalentFilters(): TalentFilters {
  return { facets: {} };
}

/** Read a catalogue filter out of URL parameters.
 *
 *  Unusable paging is OMITTED rather than forwarded, so the API applies its own default.
 *  A URL is typed by hand and pasted between people; a mistyped `limit` should land on a
 *  working page rather than on a wider answer the visitor cannot see is wider. */
export function talentFiltersFromParams(params: URLSearchParams): TalentFilters {
  const f = emptyTalentFilters();

  for (const param of TALENT_FACET_PARAMS) {
    const values = terms(params.get(param));
    if (values) f.facets[param] = values;
  }

  const minYears = boundedInt(params.get('min_years'), 0, MAX_YEARS);
  if (minYears !== undefined) f.minYears = minYears;

  const limit = boundedInt(params.get('limit'), 1, MAX_LIMIT);
  if (limit !== undefined) f.limit = limit;

  const offset = boundedInt(params.get('offset'), 0, MAX_OFFSET);
  if (offset !== undefined) f.offset = offset;

  return f;
}

/** Serialise a filter back into a URL search string.
 *
 *  An empty facet is omitted rather than written as `skills=`: the link somebody shares
 *  should not state a filter they cleared. */
export function talentFiltersToParams(f: TalentFilters): URLSearchParams {
  const params = new URLSearchParams();

  // Written in the vocabulary's own order, not the map's insertion order, so two filters
  // that select the same thing serialise to the same string — which is what makes a
  // shared link stable and a cache key meaningful.
  for (const param of TALENT_FACET_PARAMS) {
    const values = f.facets[param];
    if (values?.length) params.set(param, values.join(','));
  }

  if (f.minYears) params.set('min_years', String(f.minYears));
  if (f.limit !== undefined) params.set('limit', String(f.limit));
  // Zero is the default, so writing it only lengthens the URL.
  if (f.offset) params.set('offset', String(f.offset));

  return params;
}

/** How many facet values and scalar filters are narrowing the list, for the toolbar's
 *  badge. Paging is not a filter and is not counted. */
export function activeTalentFilterCount(f: TalentFilters): number {
  const facets = Object.values(f.facets).reduce((n, values) => n + values.length, 0);
  return facets + (f.minYears ? 1 : 0);
}

// The mutators. Each returns a NEW filter — the reactive wrapper decides when to publish
// one, and a mutator that edited in place would make that decision impossible to see.
//
// Every facet change resets paging: narrowing means "show me these", not "show me page
// four of these", and an offset kept across a change lands on an empty page that reads as
// no matches.

function withFacet(f: TalentFilters, param: string, values: string[]): TalentFilters {
  // Rebuilt by omission rather than by deleting a computed key: the same result, and it
  // keeps the object's shape something the type checker can follow.
  const { [param]: _cleared, ...rest } = f.facets;
  const facets = values.length ? { ...rest, [param]: values } : rest;
  return { ...f, facets, offset: 0 };
}

/** Pill and select: add a value, or drop it when it is already selected. */
export function toggleTalentFacet(f: TalentFilters, param: string, v: string): TalentFilters {
  const values = f.facets[param] ?? [];
  return withFacet(f, param, values.includes(v) ? values.filter((x) => x !== v) : [...values, v]);
}

/** Token input: put a value into the set; no-op on blank or a duplicate. */
export function addTalentFacet(f: TalentFilters, param: string, raw: string): TalentFilters {
  const v = raw.trim();
  const values = f.facets[param] ?? [];
  if (!v || values.includes(v)) return f;
  // Truncated rather than refused, for the same reason unusable paging is omitted: a URL
  // is typed by hand and pasted between people, and the alternative is an error page.
  if (values.length >= MAX_FILTER_TERMS) return f;
  return withFacet(f, param, [...values, v]);
}

export function removeTalentFacet(f: TalentFilters, param: string, v: string): TalentFilters {
  return withFacet(
    f,
    param,
    (f.facets[param] ?? []).filter((x) => x !== v),
  );
}

export function clearTalentFacet(f: TalentFilters, param: string): TalentFilters {
  return withFacet(f, param, []);
}

/** Years is a THRESHOLD, not a set: one value or none, so `undefined` clears it. */
export function setTalentMinYears(f: TalentFilters, years: number | undefined): TalentFilters {
  const next: TalentFilters = { ...f, offset: 0 };
  if (years === undefined) delete next.minYears;
  else next.minYears = years;
  return next;
}

/** Split a comma-joined filter, dropping blanks.
 *
 *  Returns undefined for a filter carrying no usable term, so an absent parameter and a
 *  present-but-empty one produce the same filter — matching what the API does with them.
 *  A trailing comma and stray spaces are what a UI joining chips actually emits. */
function terms(raw: string | null): string[] | undefined {
  if (!raw) return undefined;

  const parsed = raw
    .split(',')
    .map((term) => term.trim())
    .filter(Boolean);

  if (!parsed.length) return undefined;

  // The FIRST terms are kept — a filter list reads left to right, so those are the ones
  // somebody typed first.
  return parsed.slice(0, MAX_FILTER_TERMS);
}

/** Parse an integer, returning undefined for anything outside the accepted range.
 *
 *  Number() rather than parseInt(): parseInt('12abc') is 12, which would forward a
 *  parameter that is not a number at all. */
function boundedInt(raw: string | null, min: number, max: number): number | undefined {
  if (!raw) return undefined;

  const value = Number(raw);
  if (!Number.isInteger(value) || value < min || value > max) return undefined;

  return value;
}
