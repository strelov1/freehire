import { error, redirect } from '@sveltejs/kit';
import { collectionForCategory } from '$lib/collections';
import {
  categoryFromSlug,
  categorySlug,
  landingCategories,
  publishedCountries,
} from '$lib/roleLandings';
import { categoryLabel } from '$lib/labels';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The per-category country table: where this category is hiring, and what it pays
// where it hires most.
//
// This page is deliberately NOT a job feed. Fifteen categories already have a
// /collections/<category> landing, and that IS the feed — two feeds under different
// URLs would compete for one query. The split is by content: the collection answers
// "show me backend jobs", this answers "where are they, and what do they pay?".
//
// One facet call. The country distribution under the category is all the table needs,
// and the gate reads the same numbers.
export const load: PageServerLoad = async ({ params, fetch, setHeaders }) => {
  const category = categoryFromSlug(params.category);
  if (!category) error(404, 'Not found');

  // categoryFromSlug accepts any case, so /roles/Backend would render and then build
  // its canonical and links from the raw param — a second URL for one page. See the
  // pair route for the reasoning behind 308 over 404.
  const canonicalSlug = categorySlug(category);
  if (params.category !== canonicalSlug) redirect(308, `/roles/${canonicalSlug}`);

  const counts = await serverApi(fetch).facetCounts(new URLSearchParams({ category }), {
    facets: ['countries'],
  });

  const countries = publishedCountries(counts.facets.countries ?? {});
  // A category nothing publishes under is a page with an empty table — the same thin
  // page the per-pair gate refuses, so it 404s for the same reason.
  if (countries.length === 0) error(404, 'Not found');

  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  return {
    category,
    categorySlug: canonicalSlug,
    label: categoryLabel(category),
    // The curated feed for this category, where one exists. This page is the map; the
    // feed is /collections. Linking it is what keeps the two from reading as rivals
    // for one query — see the cannibalisation note in the spec.
    feed: collectionForCategory(category) ?? null,
    total: counts.total,
    countries,
    siblings: landingCategories().filter((c) => c.category !== category),
  };
};
