import { error } from '@sveltejs/kit';
import { categoryFromSlug, publishedCountries } from '$lib/roleLandings';
import { serverApi } from '$lib/server/api';
import { roleLandingPaths, urlsetXml, xmlResponse } from '$lib/sitemap';
import type { RequestHandler } from './$types';

// One category's landings, referenced by the sitemap index once per category. Lists
// only the countries that clear the publication gate, so a pair the route would 404
// never appears here — the same rule read off the same numbers.
//
// Per-category rather than one file for the whole product: ~2,200 URLs fit one
// sitemap comfortably, but assembling them in one request would mean a facet call per
// category inside it. Sharding by the axis the call is scoped to makes each shard one
// call, and the index names all of them for free.
export const GET: RequestHandler = async ({ url, fetch }) => {
  const slug = url.searchParams.get('category') ?? '';
  const category = categoryFromSlug(slug);
  if (!category) error(404, 'Not found');

  const counts = await serverApi(fetch).facetCounts(new URLSearchParams({ category }), {
    facets: ['countries'],
  });
  const countries = publishedCountries(counts.facets.countries ?? {}).map((c) => c.slug);
  // No country clears the gate → no category page either, matching the route.
  const paths = countries.length === 0 ? [] : roleLandingPaths(slug, countries);

  return xmlResponse(urlsetXml(paths.map((path) => ({ loc: `${url.origin}${path}` }))));
};
