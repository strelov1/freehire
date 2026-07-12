// The account section's navigation model: the ordered list of `my/*` sections and
// the active-item rule. Kept free of Svelte/icon imports so it stays pure and
// unit-testable; the `my/+layout.svelte` shell maps each href to its icon.

// Order = the order shown in the sidebar / tab strip. Identity (Profile) first,
// then the reading/working sections. Create actions (Submit a job, Moderation)
// deliberately live in the header menu, not here. `as const` keeps each href a
// literal route so the layout can pass it to `resolve()` type-safely (mirroring
// HeaderMenu's navLinks).
export const accountNav = [
  { href: '/my/profile', label: 'Profile' },
  { href: '/my/tracking', label: 'Tracking' },
  { href: '/my/activity', label: 'Activity' },
  { href: '/my/inbox', label: 'Inbox' },
  { href: '/my/searches', label: 'Search notifications' },
  { href: '/my/api-keys', label: 'API keys' },
  { href: '/my/submissions', label: 'My submissions' },
] as const;

export type AccountNavItem = (typeof accountNav)[number];

// A section is active when the current path equals its route or is a descendant
// of it. The trailing-slash guard means a shared string prefix that is not a path
// segment (e.g. '/my/api-keys-extra' vs '/my/api-keys') is not a match.
export function isSectionActive(path: string, href: string): boolean {
  return path === href || path.startsWith(href + '/');
}
