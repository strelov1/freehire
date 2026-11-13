<script lang="ts">
  import type { Snippet } from 'svelte';
  import { BarChart3, Key, Webhook } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { activeRouteTab } from '#lib/routeTabs.js';
  import { TabStrip, tabStripId } from '#lib/ui/index.js';
  import { api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import AccountPageHeading from '#lib/components/AccountPageHeading.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import type { ApiPlan } from '#lib/types.js';
  import { messages } from './messages';

  // The API customer's workspace. Every section opens under the same plan strip, so the
  // customer always sees which plan they hold and until when.
  let { children }: { children: Snippet } = $props();

  const TABS = [
    { id: 'usage', href: '/my/api', icon: BarChart3 },
    { id: 'keys', href: '/my/api/keys', icon: Key },
    { id: 'webhooks', href: '/my/api/webhooks', icon: Webhook },
  ] as const;
  const PANEL_ID = 'api-panel';

  const s = $derived(t(messages, locale()));
  const active = $derived(activeRouteTab(page.url.pathname, TABS, 'usage'));
  const tabs = $derived(TABS.map((tab) => ({ ...tab, label: s.tabs[tab.id], href: resolve(tab.href) })));
  const plan = new AsyncData<ApiPlan | null>(null);
  const loadPlan = () => void plan.run(() => api.getApiPlan());
  $effect(() => {
    if (isAuthenticated()) loadPlan();
  });

  const untilLine = (p: ApiPlan) => {
    const until = p.until ? new Date(p.until) : null;
    if (!until || Number.isNaN(until.getTime())) return s.planActiveNoDate;
    return format(s.planActive, {
      date: until.toLocaleDateString(locale(), { year: 'numeric', month: 'long', day: 'numeric' }),
    });
  };
</script>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex. -->
<div class="flex flex-col gap-6">
  <AccountPageHeading>{s.title}</AccountPageHeading>

  {#if plan.value?.tier}
    <section class="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-primary/40 bg-primary/5 p-4">
      <span class="rounded-md bg-primary px-2 py-0.5 text-sm font-semibold text-primary-foreground"
        >{s.tiers[plan.value.tier]}</span
      >
      <span class="text-sm text-muted-foreground">{untilLine(plan.value)}</span>
      <a href={resolve('/my/plan')} class="ml-auto text-sm font-medium underline-offset-4 hover:underline"
        >{s.manageBilling}</a
      >
    </section>
  {:else if plan.status === 'error'}
    <section class="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-border p-4" role="alert">
      <p class="text-sm text-destructive">{s.planError}</p>
      <button type="button" class="text-sm font-medium underline-offset-4 hover:underline" onclick={loadPlan}
        >{s.retry}</button
      >
    </section>
  {:else if plan.status === 'ready'}
    <section class="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-border p-4">
      <p class="text-sm text-muted-foreground">{s.noPlan}</p>
      <a href={resolve('/jobs-api')} class="text-sm font-medium underline-offset-4 hover:underline">{s.seePlans}</a>
    </section>
  {/if}

  <TabStrip {tabs} {active} label={s.tabsLabel} panelId={PANEL_ID} />

  <div id={PANEL_ID} role="tabpanel" aria-labelledby={tabStripId(PANEL_ID, active)}>
    {@render children()}
  </div>
</div>
