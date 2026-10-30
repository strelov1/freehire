// The href→Lucide-icon map for the account sections, shared by every navigation
// form (the account sidebar/tab strip in `my/+layout.svelte` and the Agent-page
// rail in `AccountNavRail.svelte`). Kept out of `accountNav.ts` so that model
// stays Svelte-free and unit-testable; this module owns the icon coupling.
//
// Keyed by the nav hrefs (`Record<AccountNavItem['href'], LucideIcon>`) so a new
// section without an icon is a compile error, not a runtime `<undefined />`.
import {
  User,
  Bot,
  LayoutList,
  Activity,
  BellRing,
  ScrollText,
  Inbox,
  Link2,
  Handshake,
  GraduationCap,
  Coins,
  Settings,
  TrendingUp,
  Plug,
  Radar,
  Building2,
  UsersRound,
  Users,
  CalendarDays,
  IdCard,
  HandHelping,
  UserPlus,
  Briefcase,
  Search,
} from '@lucide/svelte';
import type { LucideIcon } from '@lucide/svelte';
import type { AccountNavItem, WorkspaceId } from './accountNav';

export const accountNavIcons: Record<AccountNavItem['href'], LucideIcon> = {
  '/my/profile': User,
  '/my/assistant': Bot,
  '/my/cvs': ScrollText,
  // The same radar the invitation card carries, so the card and the section it opens
  // read as one feature.
  '/my/talent-network': Radar,
  '/my/referrals': Handshake,
  '/my/buddies': UsersRound,
  // Not the handshake beside it: a referral is a hand-off between two people, mentorship
  // is somebody teaching. The two sections sit together and must not read as one.
  '/my/mentorship': GraduationCap,
  // The same glyphs the mentorship tab strip carried before its panes moved here.
  '/my/mentorship/bookings': Users,
  '/my/mentorship/schedule': CalendarDays,
  '/my/mentorship/profile': IdCard,
  '/my/referrals/offers': HandHelping,
  '/my/referrals/incoming': UserPlus,
  '/my/employer/vacancies': Briefcase,
  '/my/tracking': LayoutList,
  '/my/activity': Activity,
  '/my/inbox': Inbox,
  '/my/market-pulse': TrendingUp,
  '/my/notifications': BellRing,
  '/my/integrations': Plug,
  '/my/contributions': Link2,
  '/my/employer': Building2,
  '/my/plan': Coins,
  '/my/account': Settings,
};

// One glyph per workspace, for the switcher and the sidebar's workspace heading. Each
// reuses the icon of the section that best stands for the role, so the switcher and the
// sidebar read as one thing.
export const workspaceIcons: Record<WorkspaceId, LucideIcon> = {
  seeker: Search,
  mentor: GraduationCap,
  referrer: Handshake,
  employer: Building2,
};
