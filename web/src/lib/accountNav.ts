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
  // Named lists of specific jobs — independent of the "Save" star — optionally
  // shared read-only by link. Replaces the retired public saved-search "boards".
  { href: '/my/lists', label: 'Job lists' },
  // Personal skill-demand trend: how the market for the candidate's own profile
  // skills is moving, week over week. A check-in section, not a daily one — sits
  // with the occasional group below rather than the four everyday sections above.
  // Open to everyone now that the history is backfilled (cmd/backfill-skill-history)
  // rather than starting thin.
  { href: '/my/market-pulse', label: 'Market Pulse' },
  // The agent: open to every signed-in user. It runs in our backend, and unlike the CV
  // builder below, nothing meters its spend yet.
  { href: '/my/assistant', label: 'Agent' },
  // The tailoring workspace's re-open list: CVs already tailored to a vacancy. Open to every
  // signed-in user (the plan meters the AI spend). Named after what the section is FOR — a CV
  // here is always aimed at one posting, and "CV builder" described the tool it grew out of.
  { href: '/my/cvs', label: 'Tailor' },
  // Employee referrals: request a referral, offer to refer (moderated), and — for
  // referrers — manage incoming requests. Open to every signed-in user.
  { href: '/my/referrals', label: 'Referrals' },
  // The notification center: delivery history, saved-search alerts, and the
  // account-level reminder/nudge settings, as three tabs of one section.
  { href: '/my/notifications', label: 'Notifications' },
  // Connect/disconnect surface for third-party accounts: Google (Gmail + Calendar)
  // and Telegram (the alert bot, decoupled from any one saved search). Each
  // connection's own domain page keeps a short status line pointing here instead of
  // duplicating the connect/disconnect UI.
  { href: '/my/integrations', label: 'Integrations' },
  { href: '/my/api-keys', label: 'API keys' },
  // The saved-search webhook destination: one URL + secret per account, signed
  // HTTP POSTs on the same matches email/Telegram alerts already carry.
  { href: '/my/webhook', label: 'Webhook' },
  { href: '/my/submissions', label: 'My submissions' },
  // Paste a job link we don't have yet; a supported, novel link adds a board we don't crawl.
  { href: '/my/contributions', label: 'Contributions' },
  // The plan's daily allowances and what they were spent on (analyses, CV editing,
  // contribution rewards).
  { href: '/my/plan', label: 'Plan' },
  // The account's own invite link and what it has earned. Named "Invite" and not
  // "Referrals" because that word is taken above by the employee-referral marketplace,
  // which is an unrelated feature — two sections sharing a name would be a menu nobody
  // can navigate.
  { href: '/my/invite', label: 'Invite a friend' },
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
