import { redirect } from '@sveltejs/kit';

// The page was renamed to /semantic-search; the old address is already linked from
// outside, so it keeps answering with a permanent redirect.
export const load = () => {
  redirect(301, '/semantic-search');
};
