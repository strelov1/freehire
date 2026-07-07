import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { collectionBySlug } from '$lib/collections';
import type { PageServerLoad } from './$types';

const LIMIT = 20;

// Server-render a collection's landing page: the display copy plus the first page
// of its scoped feed, so the rows are in the initial HTML. An unrecognised slug is
// a 404. The collection's facet params are pinned as JobsView `scope` on the
// client, so the feed stays fixed to the collection while the visitor can still
// filter within it.
//
// The SSR search starts from the visitor's in-URL filters, then sets the
// collection's scope params on top (scope wins) — mirroring the company page — so
// a shared or reloaded filtered collection URL (e.g. ?skills=go) server-renders
// the same filtered list the hydrated JobsView shows, not the unfiltered feed.
export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const collection = collectionBySlug(params.slug);
  if (!collection) error(404, 'Collection not found');

  const facets = new URLSearchParams(url.searchParams);
  for (const [key, value] of Object.entries(collection.params)) facets.set(key, value);

  const initial = await serverApi(fetch).searchJobs(facets, LIMIT, 0);
  return { slug: params.slug, collection, initial };
};
