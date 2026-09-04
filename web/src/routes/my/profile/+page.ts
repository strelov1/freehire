import { redirect } from '@sveltejs/kit';

// The four sections that used to be their own routes briefly went through
// `?tab=<id>` on this page before becoming routes again (see
// docs/superpowers/specs/2026-09-04-profile-section-routes-design.md) — honor any
// link into that short-lived shape so it still lands on the right section.
const KNOWN_TABS = ['contacts', 'location', 'skills', 'experience', 'education', 'screening', 'settings'];

export function load({ url }) {
  const tab = url.searchParams.get('tab');
  if (tab && KNOWN_TABS.includes(tab)) {
    redirect(308, `/my/profile/${tab}`);
  }
}
