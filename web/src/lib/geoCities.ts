import type { CityTable } from './worldMap';

// City coordinates for the jobs map: `static/geo-cities.json`, generated from Natural
// Earth's 10m populated places (public domain) — `{country: {cityKey: [lat, lng]}}`,
// keyed by the same accent-free lowercase form `placeCities` looks names up by. About
// 200 KB, so it is fetched only when the map first needs cities and then held for the
// page's life; the browser caches it across visits.
//
// A failure is memoised too. The map degrades to its countries, which are complete
// without this file, and a loader that retried would refetch on every filter change
// for as long as the network stays down.
let pending: Promise<CityTable | null> | undefined;

/** The city table, or null when it could not be loaded. Fetched at most once. */
export function loadCityTable(): Promise<CityTable | null> {
  pending ??= fetch('/geo-cities.json')
    .then((res) => (res.ok ? (res.json() as Promise<CityTable>) : null))
    .catch(() => null);
  return pending;
}
