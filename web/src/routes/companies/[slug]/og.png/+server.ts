import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import { buildCompanyCard } from '$lib/server/og/company';
import { loadOgFonts } from '$lib/server/og/fonts';
import { resolveLogo } from '$lib/server/og/logo';
import { renderMarkupPng } from '$lib/server/og/render';
import type { RequestHandler } from './$types';

// Renders the per-company 1200×630 Open Graph preview on demand. Resolves the
// company by the same slug as the detail page (a 404 there becomes a 404 here,
// with no image) and reads its live open-jobs count from the same company-scoped
// search the detail page uses. The logo fetch degrades to a monogram on any
// failure, so it never blocks the response. Cached for an hour with a day of
// stale-while-revalidate — company facts change slowly and crawlers refetch rarely.
export const GET: RequestHandler = async ({ params, fetch }) => {
  const api = serverApi(fetch);

  // The open-jobs count needs only the slug, so start it now — it runs concurrently
  // with the entity fetch instead of behind it (mirrors the company-detail loader).
  const openJobsFacets = new URLSearchParams({ company_slug: params.slug });
  const openJobsPromise = api.searchJobs(openJobsFacets, 1, 0);

  let company;
  try {
    ({ company } = await api.getCompany(params.slug, 1, 0));
  } catch (e) {
    // Company missing → mark the in-flight search handled so it isn't an
    // unhandled rejection, then surface the 404 (no image).
    openJobsPromise.catch(() => {});
    if (e instanceof ApiError && e.status === 404) error(404, 'Company not found');
    throw e;
  }

  const [openJobsSlice, fonts, logo] = await Promise.all([
    openJobsPromise,
    loadOgFonts(),
    resolveLogo(company.name),
  ]);

  const png = await renderMarkupPng(
    buildCompanyCard(company, { logo, openJobs: openJobsSlice.total ?? 0 }),
    fonts,
  );

  return new Response(png, {
    headers: {
      'content-type': 'image/png',
      'cache-control': 'public, max-age=3600, stale-while-revalidate=86400',
    },
  });
};
