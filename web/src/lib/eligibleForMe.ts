// The "Eligible for me" location filter: resolves what geography a visitor is
// plausibly eligible for, and what that resolves to in terms of the existing
// `countries`/`regions` facet values (see openspec/changes/eligible-for-me-location-filter).
//
// Pure and free of any store shape, like geoScope.ts: callers pass in what they
// already hold (a profile country, an edge-derived region, the currently-included
// facet values) rather than this module reaching into a store or the DOM itself.

import { WORLDWIDE_REGION } from './geoScope';
import { REGION_UNSPECIFIED } from './facets';

/** Where the "Eligible for me" filter resolved the visitor's geography from — a
 *  specific country (profile-level precision) or only a macro-region (edge-derived),
 *  or `null` when neither source yielded anything. */
export type GeoSource = { kind: 'country'; code: string } | { kind: 'region'; code: string } | null;

/** The two-step precedence: a signed-in profile's stated country wins when present
 *  (country-level precision); otherwise the macro-region derived from the visitor's
 *  edge country header (already resolved server-side by the existing
 *  `geo-default-scope` `/geo/region` endpoint — this function never sees a raw
 *  country code for that path). Neither present resolves to `null`. */
export function resolveGeoSource(profileCountry: string | null | undefined, edgeRegion: string | null | undefined): GeoSource {
  const country = profileCountry?.trim().toLowerCase();
  if (country) return { kind: 'country', code: country };
  const region = edgeRegion?.trim().toLowerCase();
  if (region) return { kind: 'region', code: region };
  return null;
}

/** The facet include values a `GeoSource` stages: a country source narrows to that
 *  country, a region source to that region; either way `global` and the "not
 *  specified" sentinel are always included too, so worldwide-remote postings and
 *  postings whose geography never resolved stay visible rather than reading as
 *  "not eligible". */
export interface EligibleForMeValues {
  countries: string[];
  regions: string[];
}

export function eligibleForMeValues(source: GeoSource): EligibleForMeValues {
  if (!source) return { countries: [], regions: [] };
  const safety = [WORLDWIDE_REGION, REGION_UNSPECIFIED];
  return source.kind === 'country'
    ? { countries: [source.code], regions: safety }
    : { countries: [], regions: [source.code, ...safety] };
}

/** Whether the filter reads as "on": every value the source would stage is already
 *  present among the currently-included `countries`/`regions` facet values. Derived
 *  from what is actually staged (rather than tracked as separate state) so the pill
 *  reflects the URL on reload and reads "off" the moment a visitor removes even one
 *  of the values it staged by hand. */
export function isEligibleForMeOn(source: GeoSource, includedCountries: string[], includedRegions: string[]): boolean {
  if (!source) return false;
  const values = eligibleForMeValues(source);
  return values.countries.every((c) => includedCountries.includes(c)) && values.regions.every((r) => includedRegions.includes(r));
}
