<script lang="ts">
  import type { Snippet } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { TabStrip, tabStripId } from '$lib/ui';
  import { activeNotificationTab, NOTIFICATION_TABS } from '$lib/notificationCenterTabs';
  import type { NotificationTabId } from '$lib/notificationCenterTabs';

  // The notification center's shared chrome: history, search alerts, and settings
  // are three real routes (not a client-side view switch), so the strip navigates
  // rather than only swapping local state — unlike every other TabStrip call site
  // in this codebase (/my/profile, /my/market-pulse).

  let { children }: { children: Snippet } = $props();

  const active = $derived(activeNotificationTab(page.url.pathname));
  const PANEL_ID = 'notification-center-panel';

  function onSelect(id: NotificationTabId) {
    const tab = NOTIFICATION_TABS.find((t) => t.id === id);
    if (tab) void goto(resolve(tab.href));
  }
</script>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
<div class="flex flex-col gap-6">
  <TabStrip
    tabs={NOTIFICATION_TABS}
    {active}
    {onSelect}
    label="Notification center sections"
    panelId={PANEL_ID}
  />

  <div id={PANEL_ID} role="tabpanel" aria-labelledby={tabStripId(PANEL_ID, active)}>
    {@render children()}
  </div>
</div>
