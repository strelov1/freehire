import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// Security was renamed Account once the language moved in beside the email, password and
// sessions. Old links — mail, blog posts, bookmarks — keep working through this redirect,
// query string included.
export const load: PageLoad = ({ url }) => {
  redirect(308, `/my/account${url.search}`);
};
