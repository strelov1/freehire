import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// The fit analysis moved to /match/[slug]; keep old links working with a permanent redirect.
export const load: PageLoad = ({ params }) => {
  redirect(308, `/match/${params.slug}`);
};
