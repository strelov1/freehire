import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import { loadOgFonts } from '$lib/server/og/fonts';
import { resolveLogo } from '$lib/server/og/logo';
import { renderCardPng } from '$lib/server/og/render';
import type { RequestHandler } from './$types';

// Renders the per-job 1200×630 Open Graph preview on demand. Resolves the job by
// the same slug as the detail page (a 404 there becomes a 404 here, with no image),
// then renders the card. The logo fetch degrades to a monogram on any failure, so it
// never blocks the response. Cached for an hour with a day of stale-while-revalidate
// — job content changes (enrichment, close), but crawlers refetch rarely.
export const GET: RequestHandler = async ({ params, fetch }) => {
  let job;
  try {
    job = await serverApi(fetch).getJob(params.slug);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
    throw e;
  }

  const [fonts, logo] = await Promise.all([loadOgFonts(), resolveLogo(job.company)]);
  const png = await renderCardPng(job, { fonts, logo });

  return new Response(png, {
    headers: {
      'content-type': 'image/png',
      'cache-control': 'public, max-age=3600, stale-while-revalidate=86400',
    },
  });
};
