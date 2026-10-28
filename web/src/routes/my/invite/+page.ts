import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// "Invite a friend" moved under Plan, as one of its tabs — the credit it earns is spent
// on the plan. Old links and bookmarks keep working through this redirect.
export const load: PageLoad = ({ url }) => {
  redirect(308, `/my/plan/invite${url.search}`);
};
