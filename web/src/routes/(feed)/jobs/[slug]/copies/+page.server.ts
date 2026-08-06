import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// Rows per page of the full openings-by-location list. Kept within the endpoint's cap so
// one request serves a whole page; prev/next page by offset.
const PAGE_SIZE = 100;

export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const api = serverApi(fetch);
  const offset = Math.max(0, Number(url.searchParams.get('offset') ?? '0') || 0);
  const [job, result] = await Promise.all([
    api.getJob(params.slug).catch((e) => {
      if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
      throw e;
    }),
    api.getJobCopies(params.slug, PAGE_SIZE, offset).catch(() => ({ copies: [], total: 0 })),
  ]);
  return { job, copies: result.copies, total: result.total, offset, pageSize: PAGE_SIZE };
};
