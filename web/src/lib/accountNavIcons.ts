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
  ListPlus,
  Activity,
  BellRing,
  Key,
  FileText,
  ScrollText,
  Inbox,
  Link2,
  Handshake,
  GraduationCap,
  Coins,
  Gift,
  ShieldCheck,
  TrendingUp,
  Plug,
  Radar,
  Webhook,
} from '@lucide/svelte';
import type { LucideIcon } from '@lucide/svelte';
import type { AccountNavItem } from './accountNav';

export const accountNavIcons: Record<AccountNavItem['href'], LucideIcon> = {
  '/my/profile': User,
  '/my/assistant': Bot,
  '/my/cvs': ScrollText,
  '/my/referrals': Handshake,
  // Not the handshake beside it: a referral is a hand-off between two people, mentorship
  // is somebody teaching. The two sections sit together and must not read as one.
  '/my/mentorship': GraduationCap,
  '/my/talent-network': Radar,
  '/my/tracking': LayoutList,
  '/my/lists': ListPlus,
  '/my/activity': Activity,
  '/my/inbox': Inbox,
  '/my/market-pulse': TrendingUp,
  '/my/notifications': BellRing,
  '/my/integrations': Plug,
  '/my/api-keys': Key,
  '/my/webhook': Webhook,
  '/my/submissions': FileText,
  '/my/contributions': Link2,
  '/my/plan': Coins,
  '/my/invite': Gift,
  '/my/security': ShieldCheck,
};
