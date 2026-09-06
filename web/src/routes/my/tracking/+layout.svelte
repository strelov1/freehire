<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Calendar, Columns3, List, Workflow } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { TabStrip, tabStripId } from '$lib/ui';
  import { activeRouteTab } from '$lib/routeTabs';

  let { children }: { children: Snippet } = $props();

  // The account shell (my/+layout) owns the container, auth gate, and noindex;
  // this layout adds only Tracking's own sub-navigation. Each view is its own URL
  // so it is linkable, bookmarkable, and survives a reload. Board is the index
  // route; Pipeline gets its own path. History and Matches live under Activity.
  //
  // The strip is the same underline `TabStrip` every other account section navigates
  // with, icons included.
  const SECTIONS = [
    { id: 'board', label: 'Board', href: '/my/tracking', icon: Columns3 },
    { id: 'list', label: 'List', href: '/my/tracking/list', icon: List },
    { id: 'pipeline', label: 'Pipeline', href: '/my/tracking/pipeline', icon: Workflow },
    { id: 'calendar', label: 'Calendar', href: '/my/tracking/calendar', icon: Calendar },
  ] as const;
  const PANEL_ID = 'tracking-tabpanel';

  const active = $derived(activeRouteTab(page.url.pathname, SECTIONS, 'board'));
  const tabs = $derived(SECTIONS.map((sec) => ({ ...sec, href: resolve(sec.href) })));
</script>

<svelte:head>
  <!-- Base title; the child pages override it with their view name. -->
  <title>Tracking — freehire</title>
</svelte:head>

<div class="flex flex-col gap-4">
  <h1 class="text-2xl font-semibold tracking-tight">Tracking</h1>

  <TabStrip {tabs} {active} label="Tracking view" panelId={PANEL_ID} />

  <div role="tabpanel" id={PANEL_ID} aria-labelledby={tabStripId(PANEL_ID, active)} tabindex="0">
    {@render children()}
  </div>
</div>
