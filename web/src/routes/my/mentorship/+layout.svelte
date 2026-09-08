<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { LucideIcon } from '@lucide/svelte';
  import { CalendarClock, CalendarDays, IdCard, Users } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { TabStrip, tabStripId } from '$lib/ui';
  import { MENTORSHIP_TABS, type MentorshipTabId } from '$lib/mentorshipTabs';
  import { activeRouteTab } from '$lib/routeTabs';
  import { browserTimezone } from '$lib/mentorship';
  import type { LayoutData } from './$types';

  let { data, children }: { data: LayoutData; children: Snippet } = $props();

  // Kept here rather than in mentorshipTabs.ts, which stays Svelte-free — the same split
  // accountNav/accountNavIcons makes.
  const ICONS: Record<MentorshipTabId, LucideIcon> = {
    sessions: CalendarClock,
    bookings: Users,
    profile: IdCard,
    schedule: CalendarDays,
  };

  const PANEL_ID = 'mentorship-panel';

  // The strip is for a MENTOR. A seeker has one thing in this section, and three tabs
  // about being a mentor would only make them wonder what they are missing.
  const isMentor = $derived(Boolean(data.profile));

  const active = $derived(activeRouteTab(page.url.pathname, MENTORSHIP_TABS, 'sessions'));
  const tabs = $derived(
    MENTORSHIP_TABS.map((tab) => ({ ...tab, icon: ICONS[tab.id], href: resolve(tab.href) })),
  );

  // For the subtitle only. Each pane resolves the zone itself — it is a pure call, and
  // threading it down through context would cost more indirection than the repeat.
  const timezone = browserTimezone();
</script>

<!-- The account shell (my/+layout) owns the container, auth gate and noindex. -->
<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Mentorship</h1>
    <p class="text-muted-foreground text-sm">
      Half-hour conversations with people who work where you want to. Times shown in {timezone}.
    </p>
  </div>

  {#if isMentor}
    <TabStrip {tabs} {active} label="Mentorship sections" panelId={PANEL_ID} />

    <div id={PANEL_ID} role="tabpanel" aria-labelledby={tabStripId(PANEL_ID, active)}>
      {@render children()}
    </div>
  {:else}
    <!-- No strip, so no panel to label either: an aria-labelledby pointing at a tab that
         was never rendered is worse than no landmark at all. -->
    {@render children()}
  {/if}
</div>
