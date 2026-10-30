import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// A member's page moved with the rest of interview buddies; see ../+page.server.ts.
export const load: PageServerLoad = ({ params, url }) => {
  redirect(308, `/buddies/${encodeURIComponent(params.handle)}${url.search}`);
};
