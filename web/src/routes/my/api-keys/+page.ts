import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// API keys moved under Integrations, as one of its tabs. Old links — docs, CLI help, mail,
// bookmarks — keep working through this redirect, query string included.
export const load: PageLoad = ({ url }) => {
  redirect(308, `/my/integrations/api-keys${url.search}`);
};
