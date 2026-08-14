import { redirect } from '@sveltejs/kit';

// History moved to be the notification center's landing page. 308 so bookmarks/
// inbound links (e.g. the header bell's "View all notifications") are kept.
export function load() {
  redirect(308, '/my/notifications');
}
