// The account section's navigation model: the ordered list of `my/*` sections, the
// workspace and group each belongs to, and the active-item rule. Kept free of
// Svelte/icon imports so it stays pure and unit-testable; the `my/+layout.svelte` shell
// maps each href to its icon.

// One account, four workspaces. Job search is the member as a candidate; Mentor,
// Referrer and Employer are roles they take on. The sidebar shows the CURRENT workspace
// only — the flat column of every section side by side gave no hint which of them was
// "me", which was an anonymous face, and which was a separate role. `workspace: null`
// marks the shared Settings group every workspace ends with. Workspace names and hints
// live in the i18n catalog (`i18n/shell.ts`), keyed by id.
export type WorkspaceId = 'seeker' | 'mentor' | 'referrer' | 'employer';
type NavGroupId = 'main' | 'found' | 'help' | 'settings';

const workspaces = [
  { id: 'seeker', home: '/my/profile' },
  // Beta-gated with the marketplace itself (see mentorshipGate.ts). Its home is a pane
  // only a mentor can use; for anybody else that pane redirects to the index, which is
  // where "Offer to mentor" lives.
  { id: 'mentor', home: '/my/mentorship/bookings', betaOnly: true },
  // Open to everyone: anybody may offer to refer into their employer (offers are moderated).
  { id: 'referrer', home: '/my/referrals/offers' },
  // Open to everyone: the company page starts with the claim, so it IS the way in.
  { id: 'employer', home: '/my/employer' },
] as const satisfies readonly { id: WorkspaceId; home: string; betaOnly?: boolean }[];

// Order = the order shown within each group. The four everyday sections
// lead in the order they're used — Profile (identity), Activity, Tracking, Inbox —
// then the occasional ones. Create actions
// (Submit a job, Moderation) deliberately live in the header menu, not here.
// `as const` keeps each href a literal route so the layout can pass it to `resolve()`
// type-safely (mirroring HeaderMenu's navLinks).
export const accountNav = [
  { href: '/my/profile', label: 'Profile', workspace: 'seeker', group: 'main' },
  { href: '/my/activity', label: 'Activity', workspace: 'seeker', group: 'main' },
  { href: '/my/tracking', label: 'Tracking', workspace: 'seeker', group: 'main' },
  // Mail inbox: connect Gmail and/or claim a freehire mailbox to track application
  // replies. Open to every signed-in user.
  { href: '/my/inbox', label: 'Inbox', workspace: 'seeker', group: 'main' },
  // Named lists of specific jobs — independent of the "Save" star — optionally
  // shared read-only by link. Replaces the retired public saved-search "boards".
  // Personal skill-demand trend: how the market for the candidate's own profile
  // skills is moving, week over week. A check-in section, not a daily one — sits
  // with the occasional group below rather than the four everyday sections above.
  // Open to everyone now that the history is backfilled (cmd/backfill-skill-history)
  // rather than starting thin.
  { href: '/my/market-pulse', label: 'Market Pulse', workspace: 'seeker', group: 'main' },
  // The agent: open to every signed-in user. It runs in our backend, and unlike the CV
  // builder below, nothing meters its spend yet.
  { href: '/my/assistant', label: 'Agent', workspace: 'seeker', group: 'main' },
  // The tailoring workspace's re-open list: CVs already tailored to a vacancy. Open to every
  // signed-in user (the plan meters the AI spend). Named after what the section is FOR — a CV
  // here is always aimed at one posting, and "CV builder" described the tool it grew out of.
  { href: '/my/cvs', label: 'Tailor', workspace: 'seeker', group: 'main' },
  // Being found instead of applying: the anonymous public catalogue's own settings page.
  // It used to be reachable ONLY from the profile layout's invitation card, which made it
  // invisible to anyone who never opened Profile — and that card is dismissible now, so
  // this entry is what keeps closing a banner from meaning losing the feature.
  { href: '/my/talent-network', label: 'Talent Network', workspace: 'seeker', group: 'found' },
  // Employee referrals, the member's side: asking for one. Open to every signed-in user.
  { href: '/my/referrals', label: 'Referral requests', workspace: 'seeker', group: 'help' },
  // The insider's side of referrals: what they offer and who asked. Anonymous, unlike a
  // mentor — which is why this is a workspace of its own rather than part of Mentor.
  { href: '/my/referrals/offers', label: 'Offers to refer', workspace: 'referrer', group: 'main' },
  { href: '/my/referrals/incoming', label: 'Incoming requests', workspace: 'referrer', group: 'main' },
  // Its own full-width space outside /my (the cabinet's sidebar left no room for it); the
  // entry stays so the cabinet still leads there.
  { href: '/buddies', label: 'Interview buddies', workspace: 'seeker', group: 'found' },
  // Booked mentorship sessions, upcoming and past. Beside referrals because it is the
  // same insider seen from the other end — but NOT folded into it: a referral offer is
  // anonymous on purpose and a mentor is named, which is the whole difference between
  // the two features.
  //
  // Beta-only while the marketplace has no supply: mentors are onboarded by hand, and a
  // section that opens on an empty directory reads as a broken feature rather than a new
  // one. Drop the flag once there are mentors to show.
  { href: '/my/mentorship', label: 'Mentorship', workspace: 'seeker', group: 'help', betaOnly: true },
  // The mentor's own panes. They used to be a tab strip inside Mentorship, beside the
  // member's own booked sessions — two roles on one page. `/my/mentorship` above stays
  // the member-as-mentee view, under Job search.
  { href: '/my/mentorship/bookings', label: 'Booked with you', workspace: 'mentor', group: 'main', betaOnly: true },
  { href: '/my/mentorship/schedule', label: 'Schedule', workspace: 'mentor', group: 'main', betaOnly: true },
  { href: '/my/mentorship/profile', label: 'Mentor profile', workspace: 'mentor', group: 'main', betaOnly: true },
  // The notification center: delivery history, saved-search alerts, and the
  // account-level reminder/nudge settings, as three tabs of one section.
  { href: '/my/notifications', label: 'Notifications', workspace: null, group: 'settings' },
  // Everything that connects the account to something outside it, as three tabs:
  // third-party accounts — Google (Gmail + Calendar) and Telegram (the alert bot,
  // decoupled from any one saved search) — API keys, and the saved-search webhook (one
  // URL + secret per account, signed HTTP POSTs on the same matches email/Telegram
  // alerts carry). Each connection's own domain page keeps a short status line pointing
  // here instead of duplicating the connect/disconnect UI.
  { href: '/my/integrations', label: 'Integrations', workspace: null, group: 'settings' },
  // What a member adds to the catalogue, as two tabs: a job link we don't have yet (a
  // supported, novel link adds a board we don't crawl), and the vacancies they posted.
  { href: '/my/contributions', label: 'Contributions', workspace: null, group: 'settings' },
  // Claim a company, manage its curated profile, and publish its own vacancies directly.
  // Open to every signed-in user — the claim step is what gates real access, not the nav.
  { href: '/my/employer', label: 'Company profile', workspace: 'employer', group: 'main' },
  { href: '/my/employer/vacancies', label: 'Vacancies', workspace: 'employer', group: 'main' },
  // The plan's daily allowances and what they were spent on (analyses, CV editing,
  // contribution rewards), plus a tab with the account's own invite link and what it has
  // earned. The tab is named "Invite" and not "Referrals" because that word is taken above
  // by the employee-referral marketplace, an unrelated feature.
  { href: '/my/plan', label: 'Plan', workspace: null, group: 'settings' },
  // Email, password, language and sessions: change the address, the password or the
  // interface language, and sign out everywhere when a device is lost or a session is
  // suspect. Named Account rather than Security once the language moved in; /my/security
  // redirects here.
  { href: '/my/account', label: 'Account', workspace: null, group: 'settings' },
] as const satisfies readonly {
  href: string;
  label: string;
  workspace: WorkspaceId | null;
  group: NavGroupId;
  betaOnly?: boolean;
}[];

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

// The most specific section containing the path: `/my/mentorship/schedule` lies under
// `/my/mentorship` too, and the prefix rule alone would light up both.
export function activeSection<T extends { href: string }>(path: string, items: readonly T[]): T | undefined {
  let best: T | undefined;
  for (const i of items) {
    if (isSectionActive(path, i.href) && (!best || i.href.length > best.href.length)) best = i;
  }
  return best;
}

// Read from the URL rather than remembered, so a shared link, a reload and the back button
// all land in the right workspace. Shared settings and pages with no section of their own
// fall back to Job search, the workspace every member has.
export function workspaceFor(path: string): WorkspaceId {
  return activeSection(path, accountNav)?.workspace ?? 'seeker';
}

const GROUP_ORDER: readonly NavGroupId[] = ['main', 'found', 'help', 'settings'];

// The current workspace's sections, grouped, followed by the shared settings. A group the
// gate empties is dropped rather than left as a heading over nothing.
export function workspaceNav(
  workspace: WorkspaceId,
  isModerator: boolean,
  isBetaTester: boolean,
): { group: NavGroupId; items: AccountNavItem[] }[] {
  const visible = visibleAccountNav(isModerator, isBetaTester).filter(
    (i) => i.workspace === workspace || i.workspace === null,
  );
  return GROUP_ORDER.map((group) => ({ group, items: visible.filter((i) => i.group === group) })).filter(
    (g) => g.items.length > 0,
  );
}

type Workspace = (typeof workspaces)[number];

export function visibleWorkspaces(isBetaTester: boolean): readonly Workspace[] {
  return workspaces.filter((w) => !('betaOnly' in w && w.betaOnly) || isBetaTester);
}
