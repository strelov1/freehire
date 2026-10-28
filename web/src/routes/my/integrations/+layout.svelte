<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Key, Plug, Webhook } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { TabStrip, tabStripId } from '#lib/ui/index.js';
  import { activeRouteTab } from '#lib/routeTabs.js';
  import { messages } from './messages';

  // Everything that connects the account to something outside it: third-party accounts
  // (Google, Telegram), the API keys our CLI/MCP and a member's own scripts use, and the
  // saved-search webhook. Three real routes, so each tab is linkable — the same routed
  // strip /my/profile and /my/notifications use. API keys and the webhook used to be
  // sections of their own; /my/api-keys and /my/webhook redirect here.

  let { children }: { children: Snippet } = $props();

  const TABS = [
    { id: 'connections', href: '/my/integrations', icon: Plug },
    { id: 'api-keys', href: '/my/integrations/api-keys', icon: Key },
    { id: 'webhook', href: '/my/integrations/webhook', icon: Webhook },
  ] as const;

  const PANEL_ID = 'integrations-panel';

  const s = $derived(t(messages, locale()));
  const active = $derived(activeRouteTab(page.url.pathname, TABS, 'connections'));
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
