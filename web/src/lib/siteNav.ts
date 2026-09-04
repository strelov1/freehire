// The site's top-level destinations, named once.
//
// Two navigations lead to them: the menu behind the header's burger (HeaderMenu),
// which lists all of them, and the homepage header's own row (TopBar), which on that
// one route stands in for the search box and shows a subset. Before this module they
// were two arrays, and they had already drifted — the menu's "Jobs" still pointed at
// `/` after the feed moved to `/jobs`, and nothing said so.
//
// A destination carries its own glyph, the way accountNavIcons pairs one to each
// account section, so the two navigations cannot draw the same page with different
// marks.

import {
  Bell,
  Briefcase,
  Building2,
  ChartColumn,
  Compass,
  Info,
  Layers,
  MessagesSquare,
  TrendingUp,
  Wand,
} from '@lucide/svelte';
import type { LucideIcon } from '@lucide/svelte';
import type { Pathname } from '$app/types';

export type SiteNavItem = {
  /** SvelteKit's own union of this app's route paths, not a string — so a destination
   *  that stops existing, or was never spelled right, is a compile error rather than a
   *  link that 404s. `satisfies` below checks each entry against it while keeping the
   *  literal `resolve()` needs. */
  href: Pathname;
  label: string;
  icon: LucideIcon;
};

export const NAV = {
  // The catalogue itself — what a visitor came to walk.
  jobs: { href: '/jobs', label: 'Jobs', icon: Briefcase },
  companies: { href: '/companies', label: 'Companies', icon: Building2 },
  collections: { href: '/collections', label: 'Collections', icon: Layers },

  // What this is and how it works — what a first-time visitor reads.
  howItWorks: { href: '/how-it-works', label: 'How it works', icon: Compass },
  about: { href: '/about', label: 'About', icon: Info },

  // What the product does beyond listing jobs.
  cvTailoring: { href: '/features/tailor', label: 'CV tailoring', icon: Wand },
  jobNotifications: { href: '/features/notifications', label: 'Job notifications', icon: Bell },

  // What the catalogue says about the market.
  analytics: { href: '/analytics', label: 'Analytics', icon: ChartColumn },
  trends: { href: '/trends', label: 'Trends', icon: TrendingUp },
  discussions: { href: '/discussions', label: 'Discussions', icon: MessagesSquare },
} as const satisfies Record<string, SiteNavItem>;

/** What the homepage header shows where every other page shows the search box.
 *
 *  Five, and no more: this is a shortcut to the menu's own top, not a second
 *  navigation with its own opinions about what matters. Three ways into the catalogue
 *  and two ways to find out what this is.
 *
 *  The whole row stands down below 640px, where five labels do not fit beside the
 *  brand and the burger — and where the burger is a thumb away and lists every one of
 *  these with the same glyph and a full label. */
export const HEADER_LINKS = [
  NAV.jobs,
  NAV.companies,
  NAV.collections,
  NAV.howItWorks,
  NAV.about,
] as const;
