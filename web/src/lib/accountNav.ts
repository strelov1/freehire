// The account section's navigation model: the ordered list of `my/*` sections and
// the active-item rule. Kept free of Svelte/icon imports so it stays pure and
// unit-testable; the `my/+layout.svelte` shell maps each href to its icon.

// Order = the order shown in the sidebar / tab strip. The four everyday sections
// lead in the order they're used — Profile (identity), Activity, Tracking, Inbox —
// then the occasional ones (Agent, notifications, keys, submissions). Create actions
// (Submit a job, Moderation) deliberately live in the header menu, not here.
// `as const` keeps each href a literal route so the layout can pass it to `resolve()`
// type-safely (mirroring HeaderMenu's navLinks).
export const accountNav = [
  { href: '/my/profile', label: 'Profile' },
  { href: '/my/activity', label: 'Activity' },
  { href: '/my/tracking', label: 'Tracking' },
  // Mail inbox: connect Gmail and/or claim a freehire mailbox to track application
  // replies. Open to every signed-in user.
  { href: '/my/inbox', label: 'Inbox' },
  // The agent: open to every signed-in user. It runs in our backend, and unlike the CV
  // builder below, nothing meters its spend yet.
  { href: '/my/assistant', label: 'Agent' },
  // CV builder + AI tailoring: open to every signed-in user (credits meter the AI spend).
  { href: '/my/cvs', label: 'CV builder' },
  // Employee referrals: request a referral, offer to refer (moderated), and — for
  // referrers — manage incoming requests. Open to every signed-in user.
  { href: '/my/referrals', label: 'Referrals' },
  { href: '/my/searches', label: 'Search notifications' },
  { href: '/my/api-keys', label: 'API keys' },
  { href: '/my/submissions', label: 'My submissions' },
  // Paste a job link we don't have yet; a supported, novel link earns AI credits.
  { href: '/my/contributions', label: 'Contributions' },
  // The AI-credits balance and the transaction history (grants, match/tailor debits,
  // contribution rewards).
  { href: '/my/credits', label: 'Credits' },
  // Password and sessions: change the password, and sign out everywhere when a
  // device is lost or a session is suspect.
  { href: '/my/security', label: 'Security' },
] as const;

export type AccountNavItem = (typeof accountNav)[number];

// The sections visible to a caller: a `moderatorOnly` section needs the moderator
// role, a `betaOnly` section needs beta membership (an independent group), and a
// `moderatorOrBeta` section needs either. This is a UI affordance only — the relevant
// server surfaces re-check on every request, so hiding the nav is not the security
// boundary.
export function visibleAccountNav(
  isModerator: boolean,
  isBetaTester: boolean,
): readonly AccountNavItem[] {
  return accountNav.filter((i) => {
    if ('moderatorOnly' in i && i.moderatorOnly) return isModerator;
    if ('betaOnly' in i && i.betaOnly) return isBetaTester;
    if ('moderatorOrBeta' in i && i.moderatorOrBeta) return isModerator || isBetaTester;
    return true;
  });
}

// A section is active when the current path equals its route or is a descendant
// of it. The trailing-slash guard means a shared string prefix that is not a path
// segment (e.g. '/my/api-keys-extra' vs '/my/api-keys') is not a match.
export function isSectionActive(path: string, href: string): boolean {
  return path === href || path.startsWith(href + '/');
}
