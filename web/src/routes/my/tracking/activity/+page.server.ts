import { redirect } from '@sveltejs/kit';
import { signinUrl } from '$lib/signin';
import { loadActivityYear } from '$lib/server/tracking';
import type { PageServerLoad } from './$types';

// /my/tracking/activity is a year of the caller's own job-search actions, drawn the way a
// contribution graph is. Same guard as the other tracking views; the fetch is its own,
// because Calendar answers "what happened this month" and this answers "have I been showing
// up" — a question about effort, which is why an employer's reply shades nothing here.
//
// The load fetches only. Which square an event lands on, and therefore every streak, is
// decided in the browser, the one place the reader's timezone is known — see
// activityGrid.
export const load: PageServerLoad = async ({ parent, url, fetch, request }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, signinUrl({ returnTo: url.pathname + url.search, cancelTo: '/', mode: 'login' }));
  }
  return { prefetched: await loadActivityYear(fetch, request.headers.get('cookie')) };
};
