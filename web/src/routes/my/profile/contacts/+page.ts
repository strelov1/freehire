import { redirect } from '@sveltejs/kit';

// Contacts became a tab on /my/profile instead of its own route. 308 so bookmarks/inbound
// links still land on the right section.
export function load() {
  redirect(308, '/my/profile?tab=contacts');
}
