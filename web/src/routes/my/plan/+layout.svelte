<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Coins, Gift } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { TabStrip, tabStripId } from '#lib/ui/index.js';
  import { activeRouteTab } from '#lib/routeTabs.js';
  import { messages } from './messages';

  // The plan's daily allowances, and the invite link whose credit is spent on that plan.
  // Two routed tabs, so each stays linkable; /my/invite used to be a section of its own
  // and redirects here.

  let { children }: { children: Snippet } = $props();

  const TABS = [
    { id: 'plan', href: '/my/plan', icon: Coins },
    { id: 'invite', href: '/my/plan/invite', icon: Gift },
  ] as const;

  const PANEL_ID = 'plan-panel';

  const s = $derived(t(messages, locale()));
  const active = $derived(activeRouteTab(page.url.pathname, TABS, TABS[0].id));
  const tabs = $derived(TABS.map((tab) => ({ ...tab, label: s.tabs[tab.id], href: resolve(tab.href) })));
</script>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">{s.title}</h1>
    <p class="text-sm text-muted-foreground">{s.description}</p>
  </div>

  <TabStrip {tabs} {active} label={s.tabsLabel} panelId={PANEL_ID} />

  <div id={PANEL_ID} role="tabpanel" aria-labelledby={tabStripId(PANEL_ID, active)}>
    {@render children()}
  </div>
</div>
