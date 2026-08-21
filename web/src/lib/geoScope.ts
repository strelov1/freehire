// Turning the edge's view of a request into an opening scope for the jobs feed:
// which region a country belongs to, whether the caller is a crawler, whether the
// guess may be offered at all, and what it applies.
//
// Pure by design, and free of SvelteKit and of the DOM: the endpoint imports the
// first two on the server, the feed imports the last two in the browser, and the
// unit tests run all four in plain Node. Nothing here reads a header, a store, or
// a URL — callers pass in what they already hold.

import { COUNTRY_REGION_MAP } from './generated/contracts';

/** The region value meaning "anywhere", added alongside the visitor's own region so
 *  a remote-everywhere posting is never hidden by a regional default. */
export const WORLDWIDE_REGION = 'global';

// Cloudflare's two reserved `CF-IPCountry` values. Both are shaped exactly like a
// country code and neither is a country: `XX` is "could not place this address",
// `T1` is a Tor exit. Neither appears in the region grouping, so the lookup below
// would reject them anyway — they are named here so the next reader knows the
// rejection is intended rather than incidental.
const RESERVED = new Set(['xx', 't1']);

/** The controlled region a country belongs to, or `null` when the country is
 *  missing, reserved, or outside the grouping.
 *
 *  Case matters in one direction only: the edge sends `BR`, the generated grouping
 *  is keyed `br`. Lower-casing here is the whole of the conversion. */
export function regionFromCountry(country: string | null | undefined): string | null {
  const code = country?.trim().toLowerCase();
  if (!code || RESERVED.has(code)) return null;
  return (COUNTRY_REGION_MAP as Record<string, string>)[code] ?? null;
}

/** Whether a first-time visitor should be offered the derived opening scope.
 *
 *  The whole precedence, in one place and in order:
 *
 *  - `search` — anything in the URL means they arrived through a link somebody built
 *    on purpose. Adding geography to it would show them something the sender never
 *    saw, so ANY param suppresses the guess, not only a geographic one.
 *  - `storedFilters` — they have filtered here before; that set is theirs and wins
 *    even when it names no geography at all.
 *  - `offered` — this browser has already been asked once. Only wiping browser
 *    storage re-arms it.
 */
export function shouldOfferGeoScope(state: {
  search: string;
  storedFilters: string;
  offered: boolean;
}): boolean {
  return state.search === '' && state.storedFilters === '' && !state.offered;
}

/** The query string an accepted guess applies: the visitor's region and worldwide,
 *  so a remote-everywhere posting is never hidden by a regional default. */
export function geoScopeQuery(region: string): string {
  return `regions=${region},${WORLDWIDE_REGION}`;
}

// Substrings that identify an automated client. Deliberately coarse: this decides
// whether to hand out a geography guess, so a false positive costs one visitor an
// opening scope while a false negative puts a crawler-scoped feed into an index.
//
// `Chrome-Lighthouse` matches none of these, and must not: the scheduled watchdog
// has to walk the same path a first-time visitor walks, or it measures a page
// nobody is served.
const CRAWLER = /bot|crawl|spider|slurp/i;

/** Whether a user agent belongs to an automated client. An absent user agent is
 *  treated as a browser — every real crawler names itself, and refusing the guess
 *  to anyone quiet enough to omit the header would be a rule about privacy tools,
 *  not about crawlers. */
export function isCrawler(userAgent: string | null | undefined): boolean {
  return !!userAgent && CRAWLER.test(userAgent);
}
