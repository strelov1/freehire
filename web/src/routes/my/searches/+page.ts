import { redirect } from '@sveltejs/kit';

// Saved-search alert management moved into the notification center as a tab.
// 308 so bookmarks/inbound links are kept.
export function load() {
  redirect(308, '/my/notifications/searches');
}
