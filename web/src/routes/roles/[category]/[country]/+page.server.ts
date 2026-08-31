import { error, redirect } from '@sveltejs/kit';
import { countryFromSlug, countryLabel, countrySlug } from '$lib/facets';
import { pageExists, pageOffset, parsePage } from '$lib/pagination';
import {
  categoryFromSlug,
  categorySlug,
  englishBreakdown,
  freshCount,
  isPairPublished,
  landingIntro,
  neighbourCategories,
  neighbourCountries,
  publishableSalaryBands,
} from '$lib/roleLandings';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

const LIMIT = 20;
const TOP_SKILLS = 3;

/** The axes read off the pair-scoped distribution. Counting is paid per facet and the
 *  wide-valued ones dominate, so the call names exactly what the page renders — this
 *  is the request that would otherwise count cities and companies for nothing. */
const PAIR_FACETS = [
  'skills',
  'seniority',
  'work_mode',
  'visa_sponsorship',
  'english_level',
  'reality',
  'company_size',
];

// Server-render one category × country landing.
//
// Five upstream reads, all in parallel. Three are facet counts because each answers a
// question at a different scope and no one call spans them:
//
//   byCountry  ({category})            the countries distribution — BOTH the
//                                      publication gate for this pair and the sibling
//                                      country links, which is why it is one call and
//                                      not two.
//   pair       ({category, countries}) everything the page states about the pair.
//   byCategory ({countries})           the other categories hiring in this country.
//                                      Country-scoped on purpose: the global category
//                                      ranking would recommend management and sales
//                                      here regardless of where the visitor is looking.
//
// The gate reads byCountry, never `pair.total`: the pair call carries the visitor's
// in-URL filters, so a page that exists would 404 for anyone who arrived with a filter
// narrow enough to push the filtered count under the floor.
export const load: PageServerLoad = async ({ params, url, fetch, setHeaders }) => {
  const category = categoryFromSlug(params.category);
  if (!category) error(404, 'Not found');
  const countryCode = countryFromSlug(params.country);
  if (!countryCode) error(404, 'Not found');

  // Both resolvers accept any case, so /roles/Backend/Germany would render — and then
  // build its canonical, breadcrumbs and sibling links out of the RAW params, publishing
  // a second URL for one page. Send every spelling to the one canonical form instead:
  // 308 rather than 404 because the request named a real page, and rather than merely
  // canonicalising because one URL beats two plus a pointer between them.
  const canonicalCategory = categorySlug(category);
  const canonicalCountry = countrySlug(countryCode);
  if (params.category !== canonicalCategory || params.country !== canonicalCountry) {
    redirect(308, `/roles/${canonicalCategory}/${canonicalCountry}${url.search}`);
  }

  const api = serverApi(fetch);
  const scope = { category, countries: countryCode };

  // The feed starts from the visitor's filters, then pins the scope on top (scope
  // wins) — the company/collection pattern, so a shared filtered URL server-renders
  // the list it shows rather than the unfiltered one.
  const feedFacets = new URLSearchParams(url.searchParams);
  for (const [key, value] of Object.entries(scope)) feedFacets.set(key, value);
  feedFacets.delete('page');

  const pageNumber = parsePage(url.searchParams);

  const [byCountry, pair, byCategory, salary, initial] = await Promise.all([
    api.facetCounts(new URLSearchParams({ category }), { facets: ['countries'] }),
    api.facetCounts(new URLSearchParams(scope), { facets: PAIR_FACETS }),
    api.facetCounts(new URLSearchParams({ countries: countryCode }), { facets: ['category'] }),
    api.insightsSalaryByCategoryInCountry(category, countryCode),
    api.searchJobs(feedFacets, LIMIT, pageOffset(pageNumber)),
  ]);

  const countryCounts = byCountry.facets.countries ?? {};
  if (!isPairPublished(countryCounts, countryCode)) error(404, 'Not found');

  // Past the last page there is no page, only an empty feed under a self-referencing
  // canonical — the collection landing draws the same line.
  if (!pageExists(pageNumber, initial.total)) error(404, 'Page not found');

  // A crawler burst must not become one upstream fan-out per request; the /insights
  // landings hold the same window.
  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  // The pair's own size, read off the unfiltered distribution — `pair.total` narrows
  // with the visitor's filters, and the page describes the market, not their view.
  const total = countryCounts[countryCode] ?? 0;
  const fresh = freshCount(pair.facets.reality ?? {});
  const topSkills = Object.entries(pair.facets.skills ?? {})
    .sort((a, b) => b[1] - a[1])
    .slice(0, TOP_SKILLS)
    .map(([skill]) => skill);

  return {
    category,
    // The canonical spellings, not the raw params — the page builds its canonical URL
    // and every internal link out of these, and the redirect above has already sent
    // any other spelling here.
    categorySlug: canonicalCategory,
    countryCode,
    countrySlug: canonicalCountry,
    countryLabel: countryLabel(countryCode),
    scope,
    total,
    fresh,
    intro: landingIntro({ category, countryCode, total, fresh, topSkills }),
    skills: pair.facets.skills ?? {},
    seniority: pair.facets.seniority ?? {},
    workMode: pair.facets.work_mode ?? {},
    visa: pair.facets.visa_sponsorship ?? {},
    companySize: pair.facets.company_size ?? {},
    english: englishBreakdown(pair.facets.english_level ?? {}, pair.total),
    salaryBands: publishableSalaryBands(salary),
    otherCountries: neighbourCountries(countryCounts, countryCode),
    otherCategories: neighbourCategories(byCategory.facets.category ?? {}, category),
    initial,
    pageNumber,
  };
};
