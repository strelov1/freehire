// Filters and paging for the public Talent Network catalogue live in the URL, so this
// module is the one place that converts between the two. `page.url` is the single source
// of truth — there is no separate filter store — which is what makes a narrowed catalogue
// server-rendered, navigable with the back button, and shareable as a link.

/** The catalogue's whole filter vocabulary, mirroring `talentnetwork.Query`. Every value
 *  is a closed-vocabulary term or a number; there is no free-text search, because a card
 *  carries no free text to search. */
export interface TalentQuery {
  categories?: string[];
  seniorities?: string[];
  skills?: string[];
  tz?: string[];
  cities?: string[];
  specializations?: string[];
  minYears?: number;
  limit?: number;
  offset?: number;
}

// The API's own bounds, mirrored so the UI never forwards a value the backend will refuse
// to read. The backend is the authority; these are copies, and each names what it mirrors.
//
// Getting one wrong is not cosmetic. A value the API cannot read is not an error there —
// it is reported in `meta.ignored_params` and the answer WIDENS — so a bad copy here
// shows a visitor an unfiltered catalogue while their filter chips claim otherwise.

/** internal/candidate/talentnetwork/query.go: maxLimit. */
export const MAX_LIMIT = 100;

/** internal/candidate/talentnetwork/query.go: defaultLimit. */
export const DEFAULT_LIMIT = 24;

/** internal/candidate/talentnetwork/query.go: maxFilterTerms. */
export const MAX_FILTER_TERMS = 25;

/** The widest OFFSET the API's int32 argument can carry. */
export const MAX_OFFSET = 2_147_483_647;

/** Read a catalogue query out of URL parameters.
 *
 *  Unusable paging is OMITTED rather than forwarded, so the API applies its own default.
 *  A URL is typed by hand and pasted between people; a mistyped `limit` should land on a
 *  working page rather than on a wider answer the visitor cannot see is wider. */
export function readTalentQuery(params: URLSearchParams): TalentQuery {
  const query: TalentQuery = {};

  const categories = terms(params.get('categories'));
  if (categories) query.categories = categories;

  const seniorities = terms(params.get('seniorities'));
  if (seniorities) query.seniorities = seniorities;

  const skills = terms(params.get('skills'));
  if (skills) query.skills = skills;

  const tz = terms(params.get('tz'));
  if (tz) query.tz = tz;

  const cities = terms(params.get('cities'));
  if (cities) query.cities = cities;

  const specializations = terms(params.get('specializations'));
  if (specializations) query.specializations = specializations;

  const minYears = boundedInt(params.get('min_years'), 0, 60);
  if (minYears !== undefined) query.minYears = minYears;

  const limit = boundedInt(params.get('limit'), 1, MAX_LIMIT);
  if (limit !== undefined) query.limit = limit;

  const offset = boundedInt(params.get('offset'), 0, MAX_OFFSET);
  if (offset !== undefined) query.offset = offset;

  return query;
}

/** Serialise a query back into a URL search string, for a filter control to navigate to.
 *
 *  An empty filter is omitted rather than written as `skills=`: the link somebody shares
 *  should not state a filter they cleared. */
export function writeTalentQuery(query: TalentQuery): string {
  const params = new URLSearchParams();

  if (query.categories?.length) params.set('categories', query.categories.join(','));
  if (query.seniorities?.length) params.set('seniorities', query.seniorities.join(','));
  if (query.skills?.length) params.set('skills', query.skills.join(','));
  if (query.tz?.length) params.set('tz', query.tz.join(','));
  if (query.cities?.length) params.set('cities', query.cities.join(','));
  if (query.specializations?.length) params.set('specializations', query.specializations.join(','));
  if (query.minYears) params.set('min_years', String(query.minYears));
  if (query.limit !== undefined) params.set('limit', String(query.limit));
  // Zero is the default, so writing it only lengthens the URL.
  if (query.offset) params.set('offset', String(query.offset));

  return params.toString();
}

/** Which filters can be narrowed by a control on the page. */
export type TalentFilterKey = keyof Pick<
  TalentQuery,
  'categories' | 'seniorities' | 'skills' | 'tz' | 'cities' | 'specializations'
>;

// Both helpers below return the SEARCH STRING, not a URL. The path is the caller's,
// because SvelteKit's `resolve()` owns it: a route spelled as a literal here would be
// invisible to the router's type checking and to the lint rule that enforces it.

/** The search string for the same query at a different offset, for the pager's links. */
export function talentPageSearch(query: TalentQuery, offset: number): string {
  return writeTalentQuery({ ...query, offset });
}

/** The search string for the same query with one filter's values replaced. An empty list
 *  clears that filter, and paging resets — a visitor who narrows the catalogue means
 *  "show me these", not "show me page four of these". */
export function talentFilterSearch(
  query: TalentQuery,
  key: TalentFilterKey,
  values: string[],
): string {
  // Rebuilt by spreading rather than deleting a computed key: the same result, and it
  // keeps the object's shape a thing the type checker can see.
  const { [key]: _cleared, ...rest } = query;
  const next: TalentQuery = { ...rest, offset: 0 };
  if (values.length) next[key] = values;
  return writeTalentQuery(next);
}

/** Split a comma-joined filter, dropping blanks.
 *
 *  Returns undefined for a filter carrying no usable term, so an absent parameter and a
 *  present-but-empty one produce the same query — matching what the API does with them.
 *  A trailing comma and stray spaces are what a UI joining chips actually emits. */
function terms(raw: string | null): string[] | undefined {
  if (!raw) return undefined;

  const parsed = raw
    .split(',')
    .map((term) => term.trim())
    .filter(Boolean);

  if (!parsed.length) return undefined;

  // Truncated rather than refused, for the same reason unusable paging is omitted: a URL
  // is typed by hand and pasted between people. The first terms are kept — a filter list
  // reads left to right, so those are the ones somebody typed first.
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
