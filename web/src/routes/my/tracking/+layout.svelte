<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Calendar, Columns3, Flame, List, Workflow } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { TabStrip, tabStripId } from '$lib/ui';
  import { activeRouteTab } from '$lib/routeTabs';

  let { children }: { children: Snippet } = $props();

  // The account shell (my/+layout) owns the container, auth gate, and noindex;
  // this layout adds only Tracking's own sub-navigation. Each view is its own URL
  // so it is linkable, bookmarkable, and survives a reload. Board is the index
  // route; Pipeline gets its own path.
  //
  // The five views answer five different questions, which is why none of them is a
  // mode of another: Board and List ask where each application IS, Pipeline asks
  // where they pile up, Calendar asks what happened this month, and Activity asks
  // whether the candidate has been showing up at all. That last one counts only
  // what THEY did, so its totals are deliberately smaller than Calendar's — the
  // view says so on the page rather than leaving it to be discovered.
  //
  // `/my/activity` is a different section entirely (saved jobs, history, matches,
  // hidden) and keeps that name; the two are told apart by their place in the
  // navigation, the way every other repeated label in the account shell is.
  //
  // The strip is the same underline `TabStrip` every other account section navigates
  // with, icons included.
  const SECTIONS = [
    { id: 'board', label: 'Board', href: '/my/tracking', icon: Columns3 },
    { id: 'list', label: 'List', href: '/my/tracking/list', icon: List },
    { id: 'pipeline', label: 'Pipeline', href: '/my/tracking/pipeline', icon: Workflow },
    { id: 'calendar', label: 'Calendar', href: '/my/tracking/calendar', icon: Calendar },
    { id: 'activity', label: 'Activity', href: '/my/tracking/activity', icon: Flame },
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
