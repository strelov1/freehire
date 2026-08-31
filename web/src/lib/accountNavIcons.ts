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
  Key,
  FileText,
  ScrollText,
  Inbox,
  Link2,
  Handshake,
  Coins,
  ShieldCheck,
  TrendingUp,
  Plug,
} from '@lucide/svelte';
import type { LucideIcon } from '@lucide/svelte';
import type { AccountNavItem } from './accountNav';

export const accountNavIcons: Record<AccountNavItem['href'], LucideIcon> = {
  '/my/profile': User,
  '/my/assistant': Bot,
  '/my/cvs': ScrollText,
  '/my/referrals': Handshake,
  '/my/tracking': LayoutList,
  '/my/activity': Activity,
  '/my/inbox': Inbox,
  '/my/market-pulse': TrendingUp,
  '/my/notifications': BellRing,
  '/my/integrations': Plug,
  '/my/api-keys': Key,
  '/my/submissions': FileText,
  '/my/contributions': Link2,
  '/my/plan': Coins,
  '/my/security': ShieldCheck,
};
