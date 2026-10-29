import { redirect } from '@sveltejs/kit';

// The four sections that used to be their own routes briefly went through
// `?tab=<id>` on this page before becoming routes again (see
// docs/superpowers/specs/2026-09-04-profile-section-routes-design.md) — honor any
// link into that short-lived shape so it still lands on the right section. `settings`
// is no longer a section; it goes straight to where its timezone now lives rather than
// through a second redirect (see settings/+page.ts).
const KNOWN_TABS = ['contacts', 'location', 'skills', 'experience', 'education', 'screening'];

export function load({ url }) {
  const tab = url.searchParams.get('tab');
  if (tab === 'settings') {
    redirect(308, '/my/profile/location#account-timezone');
  }
  if (tab && KNOWN_TABS.includes(tab)) {
    redirect(308, `/my/profile/${tab}`);
  }
}
