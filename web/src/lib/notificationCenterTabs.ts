// The notification center's tab structure: three real routes sharing one strip
// (`/my/notifications/+layout.svelte`). Which one a pathname is on is the shared
// rule in routeTabs.ts.

export type NotificationTabId = 'history' | 'searches' | 'settings';

// `as const` keeps each href a literal route so callers can pass it to
// `resolve()` type-safely (mirroring accountNav.ts's own use of the pattern).
export const NOTIFICATION_TABS = [
  { id: 'history', label: 'History', href: '/my/notifications' },
  { id: 'searches', label: 'Search alerts', href: '/my/notifications/searches' },
  { id: 'settings', label: 'Settings', href: '/my/notifications/settings' },
] as const satisfies readonly { id: NotificationTabId; label: string; href: string }[];
