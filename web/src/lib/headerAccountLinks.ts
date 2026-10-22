// The header's own curated account items — what the signed-in user owns/reads,
// in the same order as the account sidebar (Profile itself is rendered
// separately by each consumer, so the full run reads Profile · Activity ·
// Tracking · Inbox · …). Shared by HeaderMenu's mobile drawer and
// HeaderProfileMenu's desktop panel so the two cannot drift apart, the way
// siteNav.ts's own comment describes happening before HEADER_LINKS/NAV were
// unified.
//
// Deliberately separate from accountNavIcons.ts, which maps the FULL account
// rail (every `/my/*` section) — this is a smaller, differently-curated
// subset for the header only, and the two disagree on purpose in places (e.g.
// "Search alerts" here targets `/my/notifications/searches`, a sub-page of
// the section accountNavIcons maps as a whole).
import { Activity, BellRing, Bot, FileText, Inbox, KeyRound, ListChecks, ScrollText } from '@lucide/svelte';
import type { LucideIcon } from '@lucide/svelte';
import type { Pathname } from '#lib/pathname.js';

export type HeaderAccountLink = {
  href: Pathname;
  label: string;
  icon: LucideIcon;
};

export const accountLinks = [
  { href: '/my/activity', label: 'Activity', icon: Activity },
  { href: '/my/tracking', label: 'Tracking', icon: ListChecks },
  { href: '/my/inbox', label: 'Inbox', icon: Inbox },
  // The agent and the tailoring list are reached from anywhere, not only from the account
  // shell, so they are duplicated here beside the inbox rather than left one level deeper.
  { href: '/my/assistant', label: 'Agent', icon: Bot },
  { href: '/my/cvs', label: 'Tailor', icon: ScrollText },
  { href: '/my/notifications/searches', label: 'Search alerts', icon: BellRing },
  { href: '/my/api-keys', label: 'API keys', icon: KeyRound },
  { href: '/my/submissions', label: 'My submissions', icon: FileText },
] as const satisfies readonly HeaderAccountLink[];
