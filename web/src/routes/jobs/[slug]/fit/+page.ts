import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// The match analysis moved into the Tailor workspace; keep old links working with a permanent redirect.
export const load: PageLoad = ({ params }) => {
  redirect(308, `/tailor/${params.slug}`);
};
