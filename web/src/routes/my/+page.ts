import { redirect } from '@sveltejs/kit';

// The account area has no landing page of its own; a bare /my sends the user to
// Tracking (the first working section). 308 so bookmarks/inbound links are kept.
export function load() {
  redirect(308, '/my/tracking');
}
