import { error } from '@sveltejs/kit';
import { serverApi } from '$lib/server/api';
import { collectionBySlug, skillForCollection } from '$lib/collections';
import { pageExists, pageOffset, parsePage } from '$lib/pagination';
import { categoryLandingLink } from '$lib/roleLandings';
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
  // `page` addresses the feed, it is not a search facet — the API would ignore it,
  // but leaving it in the query would make it part of the cache key for nothing.
  facets.delete('page');

  // Server-render the page the URL asks for, not always the first: the <a href>
  // pagination beside the feed is what makes rows past the first twenty reachable
  // by a crawler at all, and each of those links has to render its own rows.
  // Named pageNumber, not page: the component side already binds `page` to
  // SvelteKit's navigation state, and two different `page`s in one file is a trap.
  const pageNumber = parsePage(url.searchParams);
  const initial = await serverApi(fetch).searchJobs(facets, LIMIT, pageOffset(pageNumber));
  // Past the last page the matches fill there is no page, only an empty feed under
  // a self-referencing canonical. parsePage clamps to MAX_PAGE rather than failing,
  // which is right for the number and wrong for what we then serve.
  if (!pageExists(pageNumber, initial.total)) error(404, 'Page not found');
  // The country map for this feed's category, where the feed pins exactly one. The
  // reverse of the link /roles/[category] carries: this page answers "show me the
  // jobs", that one answers "where are they, and what do they pay".
  const marketLink = categoryLandingLink(
    Object.keys(collection.params).length === 1 ? (collection.params.category ?? null) : null
  );
  // The definition of this feed's subject, where the feed IS one skill. Same shape as
  // marketLink above and the third side of the same division: that link answers "where
  // are they and what do they pay", this one answers "what is it", and the page itself
  // answers "who is hiring".
  //
  // No description lookup: every canonical skill carries a glossary entry (see
  // skillDescriptions.ts), and a pinned `skills` value IS a canonical facet value, so
  // the page exists by construction. Reaching for the catalog would cost this page a
  // dynamic import it has no other reason to make.
  //
  // Resolved from the slug, NOT from `collection.params`: by the time params reach here
  // scopeParams has flattened a list-valued pin to its first value, so a feed about two
  // skills would be indistinguishable from one about the first of them. skillForCollection
  // reads the raw registry, where that difference still exists.
  const glossaryLink = skillForCollection(params.slug) ?? null;
  return { slug: params.slug, collection, initial, pageNumber, marketLink, glossaryLink };
};
