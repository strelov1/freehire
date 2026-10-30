import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// Interview buddies moved out of the cabinet into its own full-width space. Links already
// sent keep working: the unread notice's `?c=<id>` opens that conversation in Chats, and
// anything else lands on the feed.
export const load: PageServerLoad = ({ url }) => {
  redirect(308, url.searchParams.has('c') ? `/buddies/chats${url.search}` : '/buddies');
};
