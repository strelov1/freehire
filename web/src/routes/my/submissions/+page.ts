import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// "My submissions" moved under Contributions, as one of its tabs. Old links and
// bookmarks keep working through this redirect, query string included.
export const load: PageLoad = ({ url }) => {
  redirect(308, `/my/contributions/vacancies${url.search}`);
};
