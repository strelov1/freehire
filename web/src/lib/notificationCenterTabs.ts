// The notification center's tab structure: three real routes sharing one strip
// (`/my/notifications/+layout.svelte`). Kept free of Svelte imports so the
// active-tab rule stays pure and unit-testable, mirroring accountNav.ts.

export type NotificationTabId = 'history' | 'searches' | 'settings';

// `as const` keeps each href a literal route so callers can pass it to
// `resolve()` type-safely (mirroring accountNav.ts's own use of the pattern).
export const NOTIFICATION_TABS = [
  { id: 'history', label: 'History', href: '/my/notifications' },
  { id: 'searches', label: 'Search alerts', href: '/my/notifications/searches' },
  { id: 'settings', label: 'Settings', href: '/my/notifications/settings' },
] as const satisfies readonly { id: NotificationTabId; label: string; href: string }[];

// The longest matching tab route wins, since '/my/notifications' (history) is a
// path-prefix of every other tab. A path that matches none (e.g. the digest
// detail page, /my/notifications/:id/jobs) falls back to history — the closest
// ancestor a reader would expect to land back on.
export function activeNotificationTab(pathname: string): NotificationTabId {
  let best: NotificationTabId = 'history';
  let bestLen = -1;
  for (const tab of NOTIFICATION_TABS) {
    const matches = pathname === tab.href || pathname.startsWith(`${tab.href}/`);
    if (matches && tab.href.length > bestLen) {
      best = tab.id;
      bestLen = tab.href.length;
    }
  }
  return best;
}
