import { redirect } from '@sveltejs/kit';
import { FILTERS_TOUCHED_COOKIE } from '$lib/filterStorage';
import { defaultFilterTarget, isCrawler } from '$lib/firstVisit';
import type { PageServerLoad } from './$types';

// The bare homepage, with nothing selected in the (feed) layout's pane. The list
// itself is server-rendered by `(feed)/+layout.server.ts` (shared with `/jobs/[slug]`);
// this load only owns the first-visit redirect, which must stay scoped to `/` — a
// shared `/jobs/[slug]` link must never be hijacked into the default filtered feed.
export const load: PageServerLoad = async ({ url, cookies, request }) => {
  const target = defaultFilterTarget({
    search: url.search,
    touched: !!cookies.get(FILTERS_TOUCHED_COOKIE),
    crawler: isCrawler(request.headers.get('user-agent')),
  });
  if (target) redirect(302, target);
};
