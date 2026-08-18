import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { backerBadges } from '$lib/backers';
import {
  collectionBySlug,
  jobFacetsFromJob,
  relatedCollectionLinks,
  type SeeAlsoCard,
} from '$lib/collections';
import { resolveSeeAlsoMark } from '$lib/seeAlsoMark';
import { serverApi } from '$lib/server/api';
import { rethrowUpstream } from '$lib/server/upstream';
import type { FacetCounts, Job } from '$lib/types';
import type { PageServerLoad } from './$types';
import { must } from '$lib/utils';

type Api = ReturnType<typeof serverApi>;

// Splits a resolved collection's params into a filter (every key but the
// last) and the one "variable" key/value a facet distribution under that
// filter can read a count off. A single-key collection's filter is simply
// empty — reading its count is then the whole-catalogue case, not a special
// one: every collection sharing the same filter (every single-key one,
// grouped under the empty filter; all ten remote+region/country combos,
// grouped under work_mode=remote) reads off ONE shared facetCounts() call, no
// matter how many of that group a job matches at once. A facet distribution
// is a marginal count per field under a filter, not a joint count across
// fields — which is why the OTHER keys have to be the filter, not also read
// from the distribution.
function splitParams(params: Record<string, string>): {
  filter: Record<string, string>;
  lastKey: string;
  lastValue: string;
} {
  const entries = Object.entries(params);
  const [lastKey, lastValue] = must(entries[entries.length - 1]);
  return { filter: Object.fromEntries(entries.slice(0, -1)), lastKey, lastValue };
}

// A missing facets[key][value] entry is never a genuine zero: every card
// reaching this is either the job's own facet/collection (so the job itself
// is ≥1) or a fallback pool entry curated to have a healthy count. It means
// Meilisearch's MaxValuesPerFacet cap dropped this value from the
// distribution — degrade to unresolved, not a false "0 jobs".
function readCount(facetsResult: FacetCounts | null, key: string, value: string): number | null {
  return facetsResult ? (facetsResult.facets[key]?.[value] ?? null) : null;
}

// The "see also" block's matched collections, each with its live open-job
// count. Kicked off the moment `job` resolves (it needs `job`'s own facets)
// so it overlaps with the page's other fetches instead of adding a second
// sequential wave after them.
async function buildSeeAlso(job: Job, api: Api): Promise<SeeAlsoCard[]> {
  const links = relatedCollectionLinks(jobFacetsFromJob(job));
  const resolvedLinks = links.map((link) => ({ link, resolved: collectionBySlug(link.slug) }));

  // Each group is one facetCounts() call and each of its cards reads exactly one
  // key off the result, so ask for only those keys. Counting every facet — the
  // endpoint's default — was ~99% of this page's render time.
  const filterGroups = new Map<string, { filter: Record<string, string>; keys: Set<string> }>();
  for (const { resolved } of resolvedLinks) {
    if (!resolved) continue;
    const { filter, lastKey } = splitParams(resolved.params);
    const groupKey = new URLSearchParams(filter).toString();
    const group = filterGroups.get(groupKey) ?? { filter, keys: new Set<string>() };
    group.keys.add(lastKey);
    filterGroups.set(groupKey, group);
  }
  const groupedFacets = new Map<string, FacetCounts | null>(
    await Promise.all(
      [...filterGroups.entries()].map(
        async ([groupKey, { filter, keys }]): Promise<[string, FacetCounts | null]> => [
          groupKey,
          await api.facetCounts(new URLSearchParams(filter), { facets: [...keys] }).catch(() => null),
        ]
      )
    )
  );

  return resolvedLinks.map(({ link, resolved }): SeeAlsoCard => {
    const backerImageSrc = backerBadges([link.slug])[0]?.mark ?? null;
    // relatedCollectionLinks already resolves every slug it returns via this
    // same collectionBySlug, so `resolved` is guaranteed here — this is TS
    // narrowing, not a real fallback path.
    if (!resolved) {
      return {
        slug: link.slug,
        title: link.title,
        count: null,
        mark: resolveSeeAlsoMark(null, backerImageSrc),
      };
    }

    const { filter, lastKey, lastValue } = splitParams(resolved.params);
    const groupKey = new URLSearchParams(filter).toString();
    const count = readCount(groupedFacets.get(groupKey) ?? null, lastKey, lastValue);
    const mark = resolveSeeAlsoMark(resolved.params, backerImageSrc);
    return { slug: link.slug, title: link.title, count, mark };
  });
}

// Server-render the job detail: fetch by slug so the article content is in the
// initial HTML. All fetches stay awaited (not streamed) so every section is in
// the SSR HTML for internal-link crawlability.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const api = serverApi(fetch);

  const jobPromise = api.getJob(params.slug).catch((e) => {
    if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
    rethrowUpstream(e);
  });
  const seeAlsoPromise = jobPromise.then((job) => buildSeeAlso(job, api));

  // Similar jobs are a non-essential discovery aid: a failure (search disabled,
  // no neighbours yet) must not break the page, so it degrades to an empty list.
  //
  // The application form is known for a minority of postings — only a few ATS platforms
  // publish one we can read — so its absence is the ordinary case, not a failure. It
  // degrades to null for the same reason the two below degrade to empty: nothing on this
  // page may be able to break the page.
  const [job, similar, copiesResult, applyForm, seeAlso] = await Promise.all([
    jobPromise,
    api.getSimilarJobs(params.slug).catch(() => []),
    // A small preview of the other-locations tab (the full list is /jobs/:slug/copies).
    // Non-essential and only meaningful for a mass-posted role, so it degrades to empty.
    api.getJobCopies(params.slug, 10).catch(() => ({ copies: [], total: 0 })),
    api.getApplyForm(params.slug).catch(() => null),
    seeAlsoPromise,
  ]);

  return {
    job,
    similar,
    copies: copiesResult.copies,
    copiesTotal: copiesResult.total,
    applyForm,
    seeAlso,
  };
};
