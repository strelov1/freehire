<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Bookmark, EyeOff, History, Sparkles } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { TabStrip, tabStripId } from '$lib/ui';
  import { activeRouteTab } from '$lib/routeTabs';
  import { messages } from '$lib/components/activity.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';

  let { children }: { children: Snippet } = $props();

  const s = $derived(t(messages, locale()));

  // The account shell (my/+layout) owns the container, auth gate, and noindex;
  // this layout adds only Activity's own sub-navigation. Each view is its own URL
  // so it is linkable, bookmarkable, and survives a reload. Saved is the index
  // route; History/Matches/Hidden get their own paths.
  //
  // The strip is the same underline `TabStrip` every other account section navigates
  // with, icons included — a sibling section of one account area reading differently
  // looks like a bug rather than a choice.
  const SECTIONS = [
    { id: 'saved', href: '/my/activity', icon: Bookmark },
    { id: 'history', href: '/my/activity/history', icon: History },
    { id: 'matches', href: '/my/activity/matches', icon: Sparkles },
    { id: 'hidden', href: '/my/activity/hidden', icon: EyeOff },
  ] as const;
  const PANEL_ID = 'activity-tabpanel';

  const active = $derived(activeRouteTab(page.url.pathname, SECTIONS, 'saved'));
  const tabs = $derived(
    SECTIONS.map((sec) => ({ ...sec, label: s.tabs[sec.id], href: resolve(sec.href) })),
  );
</script>

<svelte:head>
  <!-- Base title; the child pages override it with their view name. -->
  <title>{s.headTitle}</title>
</svelte:head>

<div class="flex flex-col gap-4">
  <h1 class="text-2xl font-semibold tracking-tight">{s.title}</h1>

  <TabStrip {tabs} {active} label={s.tablistLabel} panelId={PANEL_ID} />

  <div role="tabpanel" id={PANEL_ID} aria-labelledby={tabStripId(PANEL_ID, active)} tabindex="0">
    {@render children()}
  </div>
</div>
