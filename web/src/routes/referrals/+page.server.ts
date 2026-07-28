import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// The referrals landing moved under /features. The old URL is public — it has
// been in the sitemap and shared externally — so it keeps answering, with a
// permanent redirect that hands the ranking to the new address.
export const load: PageServerLoad = () => {
  redirect(301, '/features/referrals');
};
