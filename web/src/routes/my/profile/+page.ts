import { redirect } from '@sveltejs/kit';

// The sections that used to be their own routes briefly went through `?tab=<id>` on this
// page before becoming routes again (see
// docs/superpowers/specs/2026-09-04-profile-section-routes-design.md) — honor any link
// into that short-lived shape. Several of those sections are now parts of a bigger tab,
// so each id goes straight to where it lives today rather than through a second redirect.
// A Map, not an object literal, so `?tab=constructor` finds nothing.
const TAB_TARGETS = new Map([
  ['contacts', '/my/profile#account-contacts'],
  ['location', '/my/profile#account-location'],
  ['settings', '/my/profile#account-timezone'],
  ['skills', '/my/profile/skills'],
  ['experience', '/my/profile/experience'],
  ['education', '/my/profile/experience#account-education'],
  ['screening', '/my/profile/screening'],
]);

export function load({ url }) {
  const target = TAB_TARGETS.get(url.searchParams.get('tab') ?? '');
  if (target) redirect(308, target);
}
