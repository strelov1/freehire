import { error } from '@sveltejs/kit';
import { categoryFromSlug, landingCategories, publishedCountries } from '$lib/roleLandings';
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
    categorySlug: params.category,
    label: categoryLabel(category),
    total: counts.total,
    countries,
    siblings: landingCategories().filter((c) => c.category !== category),
  };
};
