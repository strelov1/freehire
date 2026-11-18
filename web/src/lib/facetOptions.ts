import { fuzzyMatch } from './fuzzy';
import { api } from './api';
import { sourceFamily, sourceLabel } from './sourceLabels';
import { sourceLogoUrl } from './logo';
import { type FacetOption, countryLabel, dynamicLabel } from './facets';

// Facet options built at run time: city and distribution lookups, the options a live
// distribution yields, and matching typed text against an option list. The static
// vocabularies and the facet definitions stay in facets.ts.

// Composes one /geo/cities result into a FacetOption: the value stays the bare city
// name (what a profile's location_preferences already stores), the label adds the
// country via the same countryLabel() resolver COUNTRY_OPTIONS uses, so the country a
// suggestion came from is at least readable.
export function cityOption(row: { value: string; country: string }): FacetOption {
  return { value: row.value, label: `${row.value}, ${countryLabel(row.country)}` };
}

// Option source for the profile's base-city and relocation-cities controls: a
// server-side prefix search over the embedded GeoNames dictionary (RemoteSearchSelect's
// debounce means this fires at most once per ~250ms of typing, not per keystroke).
// `country` narrows the base-city search to the already-chosen base country; the
// relocation-cities search omits it, since that set may span multiple countries.
//
export async function searchCities(query: string, country?: string): Promise<FacetOption[]> {
  return collapseCities(await api.searchCities(query, country));
}

// One city name is one option. /geo/cities answers per (name, country), so a prefix
// like "london" or "victoria" comes back several times over — and since a preference
// stores the bare name, every one of those rows would save the identical value. Listing
// them separately offered a choice that does not exist, and the repeated value collided
// in the picker's keyed {#each}, which in Svelte takes down the whole list rather than
// the duplicate row. The first row per name wins: /geo/cities returns them by
// descending population, so that is the city a reader means by the name.
export function collapseCities(rows: { value: string; country: string }[]): FacetOption[] {
  const byName = new Map<string, FacetOption>();
  for (const row of rows) if (!byName.has(row.value)) byName.set(row.value, cityOption(row));
  return [...byName.values()];
}

/** Drop options with a duplicate `value`, keeping the first. The facet controls
 *  render a keyed `{#each}` on `value`, and Svelte aborts the whole render with
 *  `each_key_duplicate` on a collision — so a single stray duplicate (e.g. a
 *  generated-vocabulary regression) would take down every page with a filter
 *  panel. This makes that failure mode a harmless repeat instead of a crash. */
export function uniqueByValue(opts: FacetOption[]): FacetOption[] {
  const seen = new Set<string>();
  return opts.filter((o) => (seen.has(o.value) ? false : seen.add(o.value)));
}

/** Read one value's count out of a distribution for DISPLAY.
 *
 *  A count arrives NEGATIVE when the endpoint holds a value's number back — the Talent
 *  Network withholds counts below a small floor (internal/candidate/talentnetwork's
 *  CountWithheld) so a single member cannot be identified by arithmetic across panes.
 *  Undefined means "show the option, no number"; rendering the sentinel as 0 would instead
 *  claim nobody carries a value the list is about to show somebody for. Endpoints that
 *  withhold nothing never emit a negative, so this is inert for them. */
export function reportedCount(n: number): number | undefined {
  return n < 0 ? undefined : n;
}

/** Brand-mark URL for a dynamic facet value, where the facet's values are entity
 *  names the logo proxy can resolve — today only `source` (see EntityLogo /
 *  sourceLogoUrl, the same pair /sources and JobSourceRow render their marks with).
 *  Other dynamic facets (skills, countries, cities) have no logo proxy to ask. */
function dynamicIcon(param: string, value: string): string | undefined {
  if (param === 'source') return sourceLogoUrl(sourceLabel(sourceFamily(value))) ?? undefined;
  return undefined;
}

/** Build select options for a dynamic facet from its live distribution (value →
 *  count) plus any already-selected values (so a selection absent from the current
 *  distribution stays listed and removable), labelled via dynamicLabel and sorted
 *  busiest-first. Shared by the filter panel's dynamic selects (FacetSection) and
 *  the onboarding stack picker. */
export function dynamicOptions(param: string, dist: Record<string, number>, selected: string[]): FacetOption[] {
  const keys = new Set<string>([...Object.keys(dist), ...selected]);
  return [...keys]
    .map((value) => ({
      value,
      label: dynamicLabel(param, value),
      count: reportedCount(dist[value] ?? 0),
      icon: dynamicIcon(param, value),
    }))
    .sort((a, b) => (b.count ?? 0) - (a.count ?? 0) || a.label.localeCompare(b.label));
}

/** Fetch one dynamic facet's live distribution and shape it into sorted typeahead
 *  options — the shared body behind skillDictionary.ts and sourceDictionary.ts (each
 *  a thin, differently-named wrapper so callers import "the skills dictionary" /
 *  "the source dictionary" rather than a bare param string). `opts.facets` narrows the
 *  request to just this facet, when the caller has no other use for the rest. Best-effort:
 *  any failure (network, decode) resolves to an empty list rather than throwing, so a
 *  caller can render "nothing to suggest yet" instead of an error. */
export async function loadFacetDistribution(param: string, opts?: { facets?: string[] }): Promise<FacetOption[]> {
  try {
    const counts = await api.facetCounts(new URLSearchParams(), opts);
    return dynamicOptions(param, counts.facets?.[param] ?? {}, []);
  } catch {
    return [];
  }
}

/** Does a facet option match the picker's search query? Always typo-tolerantly
 *  matches the display label; when `searchAliases` is supplied, also matches the
 *  option's curated shorthand aliases — the same terms that tag titles, so "swe"
 *  surfaces Software Engineering and "data engineer" finds Data Engineering. An
 *  empty query matches everything (via the label match). */
export function optionMatches(
  option: FacetOption,
  query: string,
  searchAliases?: Record<string, readonly string[]>,
): boolean {
  if (fuzzyMatch(option.label, query)) return true;
  const aliases = searchAliases?.[option.value];
  return !!aliases && aliases.some((a) => fuzzyMatch(a, query));
}

/** The options matching `query`, best match first: the label or an alias spelled exactly,
 *  then one that starts with the query, then one that contains it, then a typo-tolerant
 *  match — each tier in the order given (a busiest-first list stays busiest-first).
 *  Without tiers a typo match outranks an exact alias whenever it is the busier option:
 *  "golang" sat one typo from Solana, and Solana carried more jobs than Go. */
export function rankOptionMatches(
  candidates: readonly FacetOption[],
  query: string,
  searchAliases?: Record<string, readonly string[]>,
): FacetOption[] {
  const q = query.trim().toLowerCase();
  if (q === '') return [...candidates];
  const tier = (o: FacetOption): number => {
    const names = [o.label.toLowerCase(), ...(searchAliases?.[o.value] ?? [])];
    if (names.includes(q)) return 0;
    if (names.some((n) => n.startsWith(q))) return 1;
    if (names.some((n) => n.includes(q))) return 2;
    return optionMatches(o, q, searchAliases) ? 3 : -1;
  };
  return candidates
    .map((o) => ({ o, t: tier(o) }))
    .filter(({ t }) => t >= 0)
    .sort((a, b) => a.t - b.t)
    .map(({ o }) => o);
}
