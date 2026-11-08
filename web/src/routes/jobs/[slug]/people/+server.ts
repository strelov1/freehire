import { error, redirect } from '@sveltejs/kit';
import { ApiError } from '#lib/api.js';
import { referralPeopleUrl } from '#lib/referralPeople.js';
import { serverApi } from '#lib/server/api.js';
import { rethrowUpstream } from '#lib/server/upstream.js';
import type { Job } from '#lib/types.js';
import type { RequestHandler } from './$types';

// Look up the company only when a reader asks to find a referrer. The job page
// itself is visited mostly by crawlers and should not pay for this optional link.
export const GET: RequestHandler = async ({ params, fetch }) => {
  const api = serverApi(fetch);
  let job: Job;
  try {
    job = await api.getJob(params.slug);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) error(404, 'Job not found');
    rethrowUpstream(e);
  }

  // A missing company record still leaves the useful people-search fallback.
  const company = job.company_slug
    ? await api.getCompany(job.company_slug, 1, 0).catch(() => null)
    : null;
  const destination = referralPeopleUrl(job.company, company?.company.company_info?.linkedin);
  if (!destination) error(404, 'Company not found');
  redirect(302, destination, { external: ['https://www.linkedin.com'] });
};
