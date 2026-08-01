import { redirect } from '@sveltejs/kit';
import { loadTimeline } from '$lib/server/tracking';
import type { PageServerLoad } from './$types';

// /my/tracking/calendar is the application-event ledger laid out in time. Same guard as
// the other tracking views; the fetch is its own, because the board's rows answer "where
// is this application now" and this view answers "what happened, and when".
//
// The load fetches only. The grid is arranged in the browser, which is the only place the
// reader's timezone is known — see calendarModel.
export const load: PageServerLoad = async ({ parent, url, fetch, request }) => {
  const { user } = await parent();
  if (!user) {
    const target = url.pathname + url.search;
    redirect(302, `/?auth=required&redirect=${encodeURIComponent(target)}`);
  }
  return { prefetched: await loadTimeline(fetch, request.headers.get('cookie')) };
};
