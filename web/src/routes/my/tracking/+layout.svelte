<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Calendar, Columns3, List, Workflow } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { TabStrip, tabStripId } from '$lib/ui';

  let { children }: { children: Snippet } = $props();

  // The account shell (my/+layout) owns the container, auth gate, and noindex;
  // this layout adds only Tracking's own sub-navigation. Each view is its own URL
  // so it is linkable, bookmarkable, and survives a reload. Board is the index
  // route; Pipeline gets its own path. History and Matches live under Activity.
  //
  // The strip is the same underline `TabStrip` every other `/my/*` section navigates
  // with, icons included.
  const SECTIONS = [
    { id: 'board', label: 'Board', path: '/my/tracking', icon: Columns3 },
    { id: 'list', label: 'List', path: '/my/tracking/list', icon: List },
    { id: 'pipeline', label: 'Pipeline', path: '/my/tracking/pipeline', icon: Workflow },
    { id: 'calendar', label: 'Calendar', path: '/my/tracking/calendar', icon: Calendar },
  ] as const;
  const PANEL_ID = 'tracking-tabpanel';

  const path = $derived(page.url.pathname);
  // Board (index) matches exactly so it is not also active on the child routes.
  const active = $derived(
    SECTIONS.find((sec) => sec.path !== '/my/tracking' && path.startsWith(sec.path))?.id ?? 'board',
  );
  const tabs = $derived(
    SECTIONS.map((sec) => ({
      id: sec.id,
      label: sec.label,
      icon: sec.icon,
      href: resolve(sec.path),
    })),
  );
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
