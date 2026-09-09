// Every per-endpoint page used to live at its own URL; Scalar now owns all
// endpoint navigation from a single /docs/api page. A bookmarked or indexed
// link to one of these still resolves — to the reference as a whole — rather
// than 404ing.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async () => {
  redirect(301, '/docs/api');
};
