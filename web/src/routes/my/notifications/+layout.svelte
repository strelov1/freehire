<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { LucideIcon } from '@lucide/svelte';
  import { Bell, Settings, SearchCheck } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { TabStrip, tabStripId } from '$lib/ui';
  import { NOTIFICATION_TABS } from '$lib/notificationCenterTabs';
  import type { NotificationTabId } from '$lib/notificationCenterTabs';
  import { activeRouteTab } from '$lib/routeTabs';

  // The notification center's shared chrome: history, search alerts, and settings
  // are three real routes (not a client-side view switch), so the strip's tabs carry
  // their own `href` and navigate as links — same as /my/profile, /my/activity and
  // /my/tracking, and unlike the local view switch at /my/market-pulse.

  let { children }: { children: Snippet } = $props();

  // Kept here rather than in notificationCenterTabs.ts, which stays Svelte-free — the
  // same split accountNav/accountNavIcons makes.
  const ICONS: Record<NotificationTabId, LucideIcon> = {
    history: Bell,
    searches: SearchCheck,
    settings: Settings,
  };

  const PANEL_ID = 'notification-center-panel';

  const active = $derived(activeRouteTab(page.url.pathname, NOTIFICATION_TABS, 'history'));
  const tabs = $derived(
    NOTIFICATION_TABS.map((tab) => ({ ...tab, icon: ICONS[tab.id], href: resolve(tab.href) })),
  );
</script>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
<div class="flex flex-col gap-6">
  <!-- The section's title belongs to the layout, above the strip, the way /my/activity
       and /my/tracking title theirs — in the History pane it was the only one of the
       three routes that had a heading at all. -->
  <div class="flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Notifications</h1>
    <p class="text-sm text-muted-foreground">
      Every new-job match, reminder and application nudge you've been sent — and what you
      get next.
    </p>
  </div>

  <TabStrip {tabs} {active} label="Notification center sections" panelId={PANEL_ID} />

  <div id={PANEL_ID} role="tabpanel" aria-labelledby={tabStripId(PANEL_ID, active)}>
    {@render children()}
  </div>
</div>
