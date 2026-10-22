<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import type { ApiPlan } from '#lib/types.js';
  import { messages } from './ApiPlanPanel.messages';

  // Which API product tier the account holds, beside the keys it applies to. Only full-access
  // keys get the plan's rate; an embed key is published on other sites by design.
  const s = $derived(t(messages, locale()));

  const plan = new AsyncData<ApiPlan | null>(null);
  $effect(() => {
    if (isAuthenticated()) void plan.run(() => api.getApiPlan());
  });

  // The sentence for a held tier, with its end date when the server sent a readable one.
  const activeLine = (p: ApiPlan) => {
    if (!p.tier) return '';
    const params = { tier: s.tiers[p.tier], rate: p.per_minute.toLocaleString(locale()) };
    const until = p.until ? new Date(p.until) : null;
    if (!until || Number.isNaN(until.getTime())) return format(s.activeNoDate, params);
    const date = until.toLocaleDateString(locale(), { year: 'numeric', month: 'long', day: 'numeric' });
    return format(s.active, { ...params, date });
  };
</script>

{#if plan.value}
  <section class="flex flex-col gap-2 rounded-xl border border-border p-4">
    <h2 class="text-base font-semibold">{s.title}</h2>
    {#if plan.value.tier}
      <p class="text-sm text-muted-foreground">{activeLine(plan.value)}</p>
    {:else}
      <p class="text-sm text-muted-foreground">{s.none}</p>
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() is applied; the rule cannot see through the appended section anchor -->
      <a href={resolve('/pricing') + '#api'} class="text-sm font-medium underline-offset-4 hover:underline"
        >{s.seePlans}</a
      >
    {/if}
  </section>
{/if}
