import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// /my/tracking/[slug] renders the tracking board with the application's drawer open
// (deep-linkable — the inbox links here, and a refresh/share reopens the same card).
// The board itself is for any signed-in user; the Emails tab inside the drawer
// self-gates to moderators. Guard auth like /my/tracking and pass the slug through.
export const load: PageServerLoad = async ({ parent, params, url }) => {
  const { user } = await parent();
  if (!user) {
    redirect(302, `/?auth=required&redirect=${encodeURIComponent(url.pathname)}`);
  }
  return { slug: params.slug };
};
